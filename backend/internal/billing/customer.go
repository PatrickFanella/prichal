package billing

import (
	"context"
	"time"

	stripe "github.com/stripe/stripe-go/v87"
)

// CreateCustomer returns a first-class fixture customer ID for the original dispatch key.
// It sends no email, owner identity, organization name, payment method or metadata.
// The caller must persist dispatch reservation and its bounded retry window first.
func (a *FixtureSessions) CreateCustomer(ctx context.Context, key string) (string, error) {
	if a == nil || a.client == nil || !uuid.MatchString(key) {
		return "", ErrInvalidEvent
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	params := &stripe.CustomerCreateParams{Description: stripe.String("Managed PDS hosting fixture")}
	params.SetIdempotencyKey("pdsaas-customer-" + key)
	customer, err := a.client.V1Customers.Create(ctx, params)
	if err != nil || customer == nil || customer.Livemode || customer.Deleted || customer.Object != "customer" || !customerID.MatchString(customer.ID) {
		return "", ErrUnavailable
	}
	return customer.ID, nil
}
