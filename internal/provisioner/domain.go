package provisioner

// DomainProvisionRequest contains information needed to provision a domain
type DomainProvisionRequest struct {
	Domain    string
	User      string
	IP        string
	Subdomain string
}

// DomainProvisionResult contains the result of provisioning a domain
type DomainProvisionResult struct {
	Success      bool
	DNSZoneID    int
	PullZoneID   int
	CDNHostname  string
	Error        error
}
