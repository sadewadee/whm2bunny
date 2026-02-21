package provisioner

// SubdomainProvisionRequest contains information needed to provision a subdomain
type SubdomainProvisionRequest struct {
	Domain    string
	Subdomain string
	User      string
	IP        string
}
