package azureclients

import (
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/feiskyer/mcs/azureclients/auth"
	"github.com/feiskyer/mcs/azureclients/loadbalancerclient"
	"github.com/feiskyer/mcs/azureclients/retry"
)

// NewLoadBalancerClient creates a new LoadBalancer client.
func NewLoadBalancerClient(config *auth.AzureAuthConfig, env *azure.Environment) (loadbalancerclient.Interface, error) {
	servicePrincipalToken, err := auth.GetServicePrincipalToken(config, env)
	if err != nil {
		return nil, err
	}

	clientConfig := &retry.ClientConfig{
		CloudName:               config.Cloud,
		SubscriptionID:          config.SubscriptionID,
		ResourceManagerEndpoint: env.ResourceManagerEndpoint,
		Backoff:                 &retry.Backoff{Steps: 1},
		RateLimitConfig:         &retry.RateLimitConfig{},
		Authorizer:              autorest.NewBearerAuthorizer(servicePrincipalToken),
	}
	return loadbalancerclient.New(clientConfig), nil
}
