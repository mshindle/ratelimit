# Dynamodb Rate Tables

## RequestRate 

RequestRate limiting can be implemented in two flavors: **time** and **explicit**.

### Time Replenish

The expected usage is each rate limiter will use the same multi-tenant token and limit tables, created and managed by a separate service. However, a private token and/or limit table can be used when instantiating the middleware.

#### Token Table

The tokens for a single resource are stored in a single DynamoDB row, representing the "bucket".
The expected table schema is detailed below.

##### Attributes

These are all the expected table attributes, including the keys.

| Attribute Name | Data Type | Description                                                  |
|----------------|-----------|--------------------------------------------------------------|
| resource_name  | String    | User-defined name of the rate limited resource               |
| account_id     | String    | Id of the entity which created the resource                  |
| tokens         | Number    | Number of tokens available                                   |
| last_refill    | Number    | Timestamp, in milliseconds, when the tokens were replenished |
| last_token     | Number    | Timestamp, in milliseconds, when the last token was taken    |

*accountId* is necessary when the resource limit is on a per-user / per-client basis. For a global resource limit, a common *accountId* value should be used.

##### Keys

The key data type and description can be found in the above, attributes table.

| Attribute Name | Key Type |
|----------------|----------|
| resource_name  | HASH     |
| account_id     | RANGE    |

#### Limit Table

The limit and window for a specific account on a specific resource are stored in a single DynamoDB row.
The expected table schema is detailed below.

##### Attributes

These are all the expected table attributes, including the keys.

| Attribute Name | Data Type | Description                                                                                    |
|----------------|-----------|------------------------------------------------------------------------------------------------|
| resource_name  | String    | User-defined name of the rate limited resource                                                 |
| account_id     | String    | Id of the entity which created the resource                                                    |
| limit          | Number    | The maximum number of tokens the account may acquire on the resource                           |
| window_sec     | Number    | Sliding window of time, in seconds, wherein only the limit number of tokens will be available. |
| service_name   | String    | Name of the service that created this limit.                                                   |

##### Keys

The key data type and description can be found in the above, attributes table.

| Attribute Name | Key Type |
|----------------|----------|
| resource_name  | HASH     |
| account_id     | RANGE    |

##### Service Limits Index

The service limits global secondary index is used when updating/loading service limits.

| Attribute Name | Key Type |
|----------------|----------|
| serviceName    | HASH     |