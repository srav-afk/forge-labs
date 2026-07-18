package fleet

import "context"

type WorkerID string

type Provisioner interface {
	Provision(ctx context.Context, id ModelIdentity) (WorkerID, error)
	Retire(ctx context.Context, w WorkerID) error
	Kind() string
}
