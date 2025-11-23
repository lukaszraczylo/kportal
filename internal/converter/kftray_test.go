package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertToKPortal_SingleContext(t *testing.T) {
	kftrayConfigs := []KFTrayConfig{
		{
			Service:      "postgres",
			Namespace:    "default",
			LocalPort:    5432,
			RemotePort:   5432,
			Context:      "production",
			WorkloadType: "service",
			Protocol:     "tcp",
			Alias:        "prod-db",
		},
	}

	result := convertToKPortal(kftrayConfigs)

	assert.Len(t, result.Contexts, 1)
	assert.Equal(t, "production", result.Contexts[0].Name)
	assert.Len(t, result.Contexts[0].Namespaces, 1)
	assert.Equal(t, "default", result.Contexts[0].Namespaces[0].Name)
	assert.Len(t, result.Contexts[0].Namespaces[0].Forwards, 1)

	forward := result.Contexts[0].Namespaces[0].Forwards[0]
	assert.Equal(t, "service/postgres", forward.Resource)
	assert.Equal(t, "tcp", forward.Protocol)
	assert.Equal(t, 5432, forward.Port)
	assert.Equal(t, 5432, forward.LocalPort)
	assert.Equal(t, "prod-db", forward.Alias)
}

func TestConvertToKPortal_MultipleContexts(t *testing.T) {
	kftrayConfigs := []KFTrayConfig{
		{
			Service:      "api",
			Namespace:    "default",
			LocalPort:    8080,
			RemotePort:   8080,
			Context:      "production",
			WorkloadType: "service",
			Protocol:     "tcp",
			Alias:        "prod-api",
		},
		{
			Service:      "api",
			Namespace:    "default",
			LocalPort:    8081,
			RemotePort:   8080,
			Context:      "staging",
			WorkloadType: "service",
			Protocol:     "tcp",
			Alias:        "staging-api",
		},
	}

	result := convertToKPortal(kftrayConfigs)

	assert.Len(t, result.Contexts, 2)

	// Contexts should be sorted alphabetically
	assert.Equal(t, "production", result.Contexts[0].Name)
	assert.Equal(t, "staging", result.Contexts[1].Name)
}

func TestConvertToKPortal_MultipleNamespaces(t *testing.T) {
	kftrayConfigs := []KFTrayConfig{
		{
			Service:      "api",
			Namespace:    "default",
			LocalPort:    8080,
			RemotePort:   8080,
			Context:      "production",
			WorkloadType: "service",
			Protocol:     "tcp",
			Alias:        "api",
		},
		{
			Service:      "postgres",
			Namespace:    "database",
			LocalPort:    5432,
			RemotePort:   5432,
			Context:      "production",
			WorkloadType: "service",
			Protocol:     "tcp",
			Alias:        "db",
		},
	}

	result := convertToKPortal(kftrayConfigs)

	assert.Len(t, result.Contexts, 1)
	assert.Len(t, result.Contexts[0].Namespaces, 2)

	// Namespaces should be sorted alphabetically
	assert.Equal(t, "database", result.Contexts[0].Namespaces[0].Name)
	assert.Equal(t, "default", result.Contexts[0].Namespaces[1].Name)
}

func TestConvertToKPortal_MultipleForwardsInNamespace(t *testing.T) {
	kftrayConfigs := []KFTrayConfig{
		{
			Service:      "api",
			Namespace:    "default",
			LocalPort:    8080,
			RemotePort:   8080,
			Context:      "production",
			WorkloadType: "service",
			Protocol:     "tcp",
			Alias:        "api",
		},
		{
			Service:      "postgres",
			Namespace:    "default",
			LocalPort:    5432,
			RemotePort:   5432,
			Context:      "production",
			WorkloadType: "service",
			Protocol:     "tcp",
			Alias:        "db",
		},
		{
			Service:      "redis",
			Namespace:    "default",
			LocalPort:    6379,
			RemotePort:   6379,
			Context:      "production",
			WorkloadType: "service",
			Protocol:     "tcp",
			Alias:        "redis",
		},
	}

	result := convertToKPortal(kftrayConfigs)

	assert.Len(t, result.Contexts, 1)
	assert.Len(t, result.Contexts[0].Namespaces, 1)
	assert.Len(t, result.Contexts[0].Namespaces[0].Forwards, 3)

	// Forwards should be sorted by local port
	forwards := result.Contexts[0].Namespaces[0].Forwards
	assert.Equal(t, 5432, forwards[0].LocalPort) // postgres
	assert.Equal(t, 6379, forwards[1].LocalPort) // redis
	assert.Equal(t, 8080, forwards[2].LocalPort) // api
}

func TestConvertToKPortal_PodWorkloadType(t *testing.T) {
	kftrayConfigs := []KFTrayConfig{
		{
			Service:      "my-app",
			Namespace:    "default",
			LocalPort:    8080,
			RemotePort:   8080,
			Context:      "production",
			WorkloadType: "pod",
			Protocol:     "tcp",
			Alias:        "app",
		},
	}

	result := convertToKPortal(kftrayConfigs)

	forward := result.Contexts[0].Namespaces[0].Forwards[0]
	assert.Equal(t, "pod/my-app", forward.Resource)
}

func TestConvertToKPortal_WithoutAlias(t *testing.T) {
	kftrayConfigs := []KFTrayConfig{
		{
			Service:      "postgres",
			Namespace:    "default",
			LocalPort:    5432,
			RemotePort:   5432,
			Context:      "production",
			WorkloadType: "service",
			Protocol:     "tcp",
			Alias:        "", // No alias
		},
	}

	result := convertToKPortal(kftrayConfigs)

	forward := result.Contexts[0].Namespaces[0].Forwards[0]
	assert.Equal(t, "", forward.Alias)
}

func TestConvertToKPortal_DifferentPorts(t *testing.T) {
	kftrayConfigs := []KFTrayConfig{
		{
			Service:      "api",
			Namespace:    "default",
			LocalPort:    8080,
			RemotePort:   3000,
			Context:      "production",
			WorkloadType: "service",
			Protocol:     "tcp",
			Alias:        "api",
		},
	}

	result := convertToKPortal(kftrayConfigs)

	forward := result.Contexts[0].Namespaces[0].Forwards[0]
	assert.Equal(t, 3000, forward.Port, "Remote port should be 3000")
	assert.Equal(t, 8080, forward.LocalPort, "Local port should be 8080")
}
