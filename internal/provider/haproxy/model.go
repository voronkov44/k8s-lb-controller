package haproxy

// Config contains runtime settings for the file-based HAProxy provider.
type Config struct {
	ConfigPath      string
	ValidateCommand string
	ReloadCommand   string
}

const (
	configPlaceholder = "{{config}}"
	configHeader      = `# Managed by k8s-lb-controller. DO NOT EDIT.
global
    maxconn 2048

defaults
    mode tcp
    timeout connect 5s
    timeout client 30s
    timeout server 30s
`
)
