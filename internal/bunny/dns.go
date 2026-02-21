package bunny

// DNSRecordType represents the type of DNS record
type DNSRecordType int

const (
	// DNSRecordTypeA represents an A record
	DNSRecordTypeA DNSRecordType = 0
	// DNSRecordTypeAAAA represents an AAAA record
	DNSRecordTypeAAAA DNSRecordType = 1
	// DNSRecordTypeCNAME represents a CNAME record
	DNSRecordTypeCNAME DNSRecordType = 2
	// DNSRecordTypeTXT represents a TXT record
	DNSRecordTypeTXT DNSRecordType = 3
	// DNSRecordTypeMX represents an MX record
	DNSRecordTypeMX DNSRecordType = 4
)

// DNSRecord represents a DNS record
type DNSRecord struct {
	ID       int
	Type     DNSRecordType
	Name     string
	Value    string
	TTL      int
	Priority int
}
