package gateway

type SelectedWorker struct {
	ID       string
	Endpoint string
	Models   []string
}

type WorkerSelector interface {
	SelectWorker(model, prompt string) (*SelectedWorker, error)
	ListModels() []modelObject
}
