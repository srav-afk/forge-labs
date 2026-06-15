package registry

import registryv1 "github.com/srav-afk/forge-labs/gen/registry/v1"

type RegistryService interface {
	registryv1.RegistryServiceServer
}
