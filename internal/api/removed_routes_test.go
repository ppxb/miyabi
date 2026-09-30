package api

import "testing"

func TestRemovedRoutesLeaveActiveWorkflowsRegistered(t *testing.T) {
	router := NewRouter(Dependencies{Access: NewAccessGateService("", "")})
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"POST /api/auth/logout",
		"POST /api/library/scan/local",
		"GET /api/discover/movies/:id/offline",
	} {
		if routes[route] {
			t.Errorf("unused route still registered: %s", route)
		}
	}
	for _, route := range []string{
		"POST /api/auth/login",
		"POST /api/library/scan",
		"POST /api/discover/movies/:id/offline",
		"GET /api/offline/tasks",
	} {
		if !routes[route] {
			t.Errorf("active route removed: %s", route)
		}
	}
}
