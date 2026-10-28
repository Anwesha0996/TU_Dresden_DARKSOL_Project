package aap2_client

import aap2 "github.com/darksol-cloud/helm-dtn/pkg/proto"

// Struct for allowing developers to override the default
// configuration for an AAP2 connection
//
// Example Usage:
//
//	yourClient.Configure(aap2_client.WithAgentId("testagent"))
//
// It is possible to provide all, some, or none of the configuration
// options to AAP2SyncClient.Configure(). The order in which the configuration
// options are supplied does not matter.
type AAP2ConfigurationOptions struct {
	AgentId          string
	Subscribe        bool
	Secret           string
	AuthType         aap2.AuthType
	KeepaliveSeconds uint32
}

type AAP2ConfigurationOption func(*AAP2ConfigurationOptions)

// WithAgentId instructs sets the agent ID the client requests when registering.
func WithAgentId(agentId string) AAP2ConfigurationOption {
	return func(aco *AAP2ConfigurationOptions) {
		aco.AgentId = agentId
	}
}

// WithSubscribe sets a boolean indicating whether the client will be
// used for sending or receiving data to/from µD3TN.
// If `true`, the direction of control is switched after positive
// confirmation, i.e., the Server becomes the Initiator of all following
// communication and also takes over sending the Keepalive messages.
func WithSubscribe(subscribe bool) AAP2ConfigurationOption {
	return func(aco *AAP2ConfigurationOptions) {
		aco.Subscribe = subscribe
	}
}

// WithSecrect sets the secret the client will use to authenticate against µD3TN.
// When creating a new registration, this field is optional, but only if it is set,
// the same registration can be claimed by other connections (that need to specify the same secret).
func WithSecret(secret string) AAP2ConfigurationOption {
	return func(aco *AAP2ConfigurationOptions) {
		aco.Secret = secret
	}
}

// WithAuthType sets the AuthType, which controls the actions allowed and data
// provided over the connection.
func WithAuthType(authType aap2.AuthType) AAP2ConfigurationOption {
	return func(aco *AAP2ConfigurationOptions) {
		aco.AuthType = authType
	}
}

// WithKeepalive sets the maximum time interval between messages.
// It should be zero, or between 30 sec and 600 sec.
// The Initiator SHALL send `KEEPALIVE` messages after this amount of time has elapsed.
// 0 disables the feature (see RFC 9174), which is useful e.g. for local sockets.
// Default = 0 (disabled).
func WithKeepalive(keepaliveSeconds uint32) AAP2ConfigurationOption {
	return func(aco *AAP2ConfigurationOptions) {
		aco.KeepaliveSeconds = keepaliveSeconds
	}
}
