package dashboard

import "dfipam/api/internal/auth"

func holdsAny(p *auth.Principal, permissions ...string) bool {
	for _, name := range permissions {
		if p.Has(name) {
			return true
		}
	}
	return false
}

// limitTo clears the totals of areas this person can't open, so the API says no more
// than the pages do. Cleared totals are left out of the response.
func (s *Summary) limitTo(p *auth.Principal) {
	if !holdsAny(p, auth.IPRequest, auth.RequestsApprove, auth.IPAllocate, auth.IPReserve, auth.AuditView) {
		s.PendingApprovals, s.ReadyToFulfil = 0, 0
	}
	if !holdsAny(p, auth.IntegrationsManage, auth.AuditView) {
		s.IntegrationsOK, s.IntegrationsBad = 0, 0
	}
	if !holdsAny(p, auth.DiscoveryRun, auth.DiscoveryManage, auth.AuditView) {
		s.AgentsOnline, s.AgentsOffline, s.JobsRunning, s.JobsFailed = 0, 0, 0, 0
	}
}
