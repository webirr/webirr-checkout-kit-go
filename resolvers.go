package checkout

import "context"

// GatewayPollingStatusResolver calls WeBirr GetPaymentStatus through the official SDK.
type GatewayPollingStatusResolver struct{}

func (GatewayPollingStatusResolver) ResolveStatus(ctx context.Context, checkout *Checkout, payable Payable) (CheckoutStatusResult, error) {
	return checkout.ResolveGatewayStatus(ctx, payable)
}

// LocalStatusResolver reads status from the merchant's local store only.
type LocalStatusResolver struct{}

func (LocalStatusResolver) ResolveStatus(ctx context.Context, checkout *Checkout, payable Payable) (CheckoutStatusResult, error) {
	return checkout.ResolveLocalStatus(ctx, payable)
}

// HybridStatusResolver reads local status first and calls WeBirr only while local state is pending or unknown.
type HybridStatusResolver struct{}

func (HybridStatusResolver) ResolveStatus(ctx context.Context, checkout *Checkout, payable Payable) (CheckoutStatusResult, error) {
	local, err := checkout.ResolveLocalStatus(ctx, payable)
	if err != nil {
		return CheckoutStatusResult{}, err
	}
	if local.Status == StatusPaid || local.Status == StatusFailed {
		return local, nil
	}
	return checkout.ResolveGatewayStatus(ctx, payable)
}
