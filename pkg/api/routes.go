package api

func (h *Handler) GetRoutes() []Route {
	return []Route{
		{
			"GetNodes",
			"GET",
			"/node",
			h.GetNodes,
		},
		{
			"GetPools",
			"GET",
			"/pool",
			h.GetPools,
		},
		{
			"CreatePool",
			"POST",
			"/pool",
			h.CreatePool,
		},
		{
			"GetMonitors",
			"GET",
			"/mon",
			h.GetMonitors,
		},
	}
}
