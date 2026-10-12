package engine

type ServiceState string

const (
	NotInstalled ServiceState = "NotInstalled"
	Installing   ServiceState = "Installing"
	Installed    ServiceState = "Installed"
	Running      ServiceState = "Running"
	Stopped      ServiceState = "Stopped"
	Failed       ServiceState = "Failed"
	Skipped      ServiceState = "Skipped"
	Updating     ServiceState = "Updating"
	Uninstalling ServiceState = "Uninstalling"
)
