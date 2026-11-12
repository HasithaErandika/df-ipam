// Package platforms lists the operating systems and server services a person can
// record. The web app draws an icon for each; the lists here keep both in step.
package platforms

import "slices"

// Platforms are operating systems and appliance families.
var Platforms = []string{
	"windows", "windows_server", "macos", "ios", "android", "linux", "ubuntu", "debian", "redhat", "centos",
	"rocky", "alma", "fedora", "freebsd", "chromeos", "vmware", "cisco", "fortinet", "mikrotik", "ubiquiti",
	"huawei", "hp", "epson", "synology", "raspberrypi", "other",
}

// Services are what a server mainly runs.
var Services = []string{
	"apache", "nginx", "tomcat", "iis", "docker", "postgresql", "mysql", "mssql", "oracle", "dns", "dhcp",
	"mail", "file", "active_directory", "backup", "other",
}

// ValidPlatform allows the empty value, meaning not recorded.
func ValidPlatform(p string) bool { return p == "" || slices.Contains(Platforms, p) }

// CleanServices removes duplicates and reports the first unknown service.
func CleanServices(in []string) ([]string, string) {
	out := []string{}
	for _, s := range in {
		if !slices.Contains(Services, s) {
			return nil, s
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out, ""
}
