package provisioner

// DeprovisionRequest contains information needed to deprovision a domain
type DeprovisionRequest struct {
	Domain string
}

// DeprovisionResult contains the result of deprovisioning a domain
type DeprovisionResult struct {
	Success bool
	Error   error
}
