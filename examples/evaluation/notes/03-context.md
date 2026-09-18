# Canceling HTTP work

The example worker uses context.WithTimeout to bound a request.
It defers cancel and passes the context into http.NewRequestWithContext.
When the caller cancels, the request can stop instead of waiting for the entire timeout.
