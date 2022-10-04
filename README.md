# ratelimit

A rate-limiter module which borrows on ideas from [Stripe](https://stripe.com/blog/rate-limiters) and the [rate-limiter](https://github.com/lifeomic/rate-limiter-py) python module.

## RequestRate

Use a basic token bucket algorithm to 
determine if a resource is available for use. If the bucket is empty, the resource is marked unavailable.

There are two forms of limiting in this manner. 
1. **Time Replenish** Tokens are automatically added to the bucket over a period time. Typical use-cases center around a resource only allowing usage at a pre-determined rate (i.e. 50 requests / second).
2. **Explicit Release** Tokens need to be explicitly returned as the underlying resource might have a highly variable run-length time.

