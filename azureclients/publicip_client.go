package azureclients

import (
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/feiskyer/mcs/azureclients/auth"
	"github.com/feiskyer/mcs/azureclients/publicipclient"
	"github.com/feiskyer/mcs/azureclients/retry"
)

// NewPublicIPClient creates a new PublicIP client.
func NewPublicIPClient(config *auth.AzureAuthConfig, env *azure.Environment) (publicipclient.Interface, error) {
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
	return publicipclient.New(clientConfig), nil
}
