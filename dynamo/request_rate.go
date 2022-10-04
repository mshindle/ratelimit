package dynamo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/mshindle/ratelimit"
)

const (
	tokenCol      = "tokens"
	lastRefillCol = "last_refill"
	lastTokenCol  = "last_token"
)

type RequestRateLimit struct {
	ResourceName string  `dynamodbav:"resource_name"`
	AccountID    string  `dynamodbav:"account_id"`
	Limit        int64   `dynamodbav:"limit"`
	WindowSec    float64 `dynamodbav:"window_sec"`
	ServiceName  string  `dynamodbav:"service_name"`
}

type RequestRate struct {
	ResourceName string `dynamodbav:"resource_name"`
	AccountID    string `dynamodbav:"account_id"`
	Tokens       int64  `dynamodbav:"tokens"`
	LastRefill   int64  `dynamodbav:"last_refill"`
	LastToken    int64  `dynamodbav:"last_token"`
}

func (rl *RateLimiter) SetLimit(ctx context.Context, resourceName, accountID string, limit int64, windowSec float64) error {
	rrl := &RequestRateLimit{
		ResourceName: resourceName,
		AccountID:    accountID,
		Limit:        limit,
		WindowSec:    windowSec,
		ServiceName:  rl.serviceName,
	}
	av, err := attributevalue.MarshalMap(rrl)
	if err != nil {
		return fmt.Errorf("failed to marshal request rate limit: %w", err)
	}

	if rl.debug {
		encoder := json.NewEncoder(rl.writer)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(av)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(rl.limitTable),
		Item:      av,
	}
	_, err = rl.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("save failed: %w", err)
	}

	return nil
}

// GetLimit retrieves the limit and window for an account on this resource.
// If no limits for the given account and this resource is found in DynamoDB a default
// limit and window of 1000 and 1 respectively will be returned.
func (rl *RateLimiter) GetLimit(ctx context.Context, resourceName, accountID string) (int64, float64, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(rl.limitTable),
		Key: map[string]types.AttributeValue{
			limitKey:   &types.AttributeValueMemberS{Value: resourceName},
			limitRange: &types.AttributeValueMemberS{Value: accountID},
		},
	}

	out, err := rl.client.GetItem(ctx, input)
	if err != nil {
		return 0, 0.0, err
	}

	rrLimit := &RequestRateLimit{Limit: defaultLimitRate, WindowSec: defaultWindowSecond}
	if out.Item != nil {
		err = attributevalue.UnmarshalMap(out.Item, rrLimit)
		if err != nil {
			return 0, 0.0, fmt.Errorf("failed to unmarshal RequestRateLimit: %w", err)
		}
	}
	return rrLimit.Limit, rrLimit.WindowSec, nil
}

// GetToken retrieves a time replenished token for the specific account on this resource.
// If a token was successfully retrieved, the number of tokens accumulated since the last refill
// will be added back to the balance.
// If the account has reached its limit, the function will return false.
func (rl *RateLimiter) GetToken(ctx context.Context, resourceName, accountID string) (bool, error) {
	startTime := time.Now().UnixMilli() // startTime in milliseconds

	limit, windowSec, err := rl.GetLimit(ctx, resourceName, accountID)
	if err != nil {
		return false, err
	}

	windowMS := windowSec * 1000.0
	tokenMS := float64(limit) / windowMS                     // number of tokens bucket gains per ms
	msToken := int64(math.Max(1.0, windowMS/float64(limit))) // number of milliseconds to accumulate a new token
	atts, err := rl.getBucketTokens(ctx, resourceName, accountID, startTime, msToken)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			if rl.debug {
				_, _ = fmt.Fprintf(rl.writer, "capacity exhausted for %s on %s: %v ", resourceName, accountID, ccf)
			}
			return false, ratelimit.ErrCapacityReached
		}
		return false, err
	}

	var currentTokens int64
	if ct, ok := atts[tokenCol].(*types.AttributeValueMemberN); ok {
		currentTokens, err = strconv.ParseInt(ct.Value, 10, 64)
		if err != nil && rl.debug {
			_, _ = fmt.Fprintf(rl.writer, "tokens value could not be parsed: %v", ct.Value)
		}
	}
	if rl.debug {
		_, _ = fmt.Fprintf(rl.writer, "current token value: %v", currentTokens)
	}

	var lastRefill int64 = 0 // set lastRefill to 0 in case row doesn't exist yet...
	if lr, ok := atts[lastRefillCol].(*types.AttributeValueMemberN); ok {
		lastRefill, err = strconv.ParseInt(lr.Value, 10, 64)
		if err != nil && rl.debug {
			_, _ = fmt.Fprintf(rl.writer, "last_refill value could not be parsed: %v", lr.Value)
		}
	}
	timeSinceRefill := startTime - lastRefill

	refillTokens := rl.computeTokenRefillAmount(currentTokens, timeSinceRefill, limit, tokenMS)
	if rl.debug {
		_, _ = fmt.Fprintf(rl.writer, "refill token value: %v", refillTokens)
	}
	err = rl.refillBucketTokens(ctx, resourceName, accountID, refillTokens, startTime)
	if rl.debug {
		_, _ = fmt.Fprintf(rl.writer, "error refilling tokens: %v", err)
	}

	return true, nil
}

func (rl *RateLimiter) getBucketTokens(ctx context.Context,
	resourceName, accountID string, startTime, msToken int64) (map[string]types.AttributeValue, error) {

	expr, err := expression.NewBuilder().
		WithUpdate(
			expression.
				Add(expression.Name(tokenCol), expression.Value(-1)).
				Set(expression.Name(lastTokenCol), expression.Value(startTime)),
		).
		WithCondition(
			expression.Or(
				expression.AttributeNotExists(expression.Name(tokenCol)),
				expression.Or(
					expression.Name(tokenCol).GreaterThan(expression.Value(0)),
					expression.Name(lastTokenCol).LessThan(expression.Value(startTime-msToken))),
			),
		).
		Build()
	if err != nil {
		return nil, err
	}

	input := &dynamodb.UpdateItemInput{
		Key: map[string]types.AttributeValue{
			tokenKey:   &types.AttributeValueMemberS{Value: resourceName},
			tokenRange: &types.AttributeValueMemberS{Value: accountID},
		},
		TableName:                 aws.String(rl.tokenTable),
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ReturnValues:              "ALL_NEW",
	}

	out, err := rl.client.UpdateItem(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.Attributes, nil
}

func (rl *RateLimiter) refillBucketTokens(ctx context.Context, resourceName, accountID string, tokens, refillTime int64) error {
	expr, err := expression.NewBuilder().
		WithUpdate(
			expression.
				Set(expression.Name(tokenCol), expression.Value(tokens)).
				Set(expression.Name(lastRefillCol), expression.Value(refillTime)),
		).
		WithCondition(
			expression.Or(
				expression.AttributeNotExists(expression.Name(lastRefillCol)),
				expression.Name(lastRefillCol).LessThan(expression.Value(refillTime))),
		).
		Build()
	if err != nil {
		return err
	}

	input := &dynamodb.UpdateItemInput{
		Key: map[string]types.AttributeValue{
			tokenKey:   &types.AttributeValueMemberS{Value: resourceName},
			tokenRange: &types.AttributeValueMemberS{Value: accountID},
		},
		TableName:                 aws.String(rl.tokenTable),
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ReturnValues:              "NONE",
	}

	_, err = rl.client.UpdateItem(ctx, input)
	return err
}

func (rl *RateLimiter) computeTokenRefillAmount(currentTokens, timeSinceRefill, limit int64, tokenMS float64) int64 {
	var t int64
	// tokens can be negative on bucket creation or a prolonged failure to refill - so default to 0
	if currentTokens > t {
		t = currentTokens
	}

	t = t + int64(tokenMS*float64(timeSinceRefill))
	if t > limit-1 {
		return limit - 1
	}
	return t
}
