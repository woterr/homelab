package api

import (
	"net/http"
)

// handleSystem returns the combined host snapshot.
func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	info := s.sources.System.Info()
	var cpu any
	if s.sources.CPU != nil {
		cpu = s.sources.CPU.CPU()
	}
	var mem any
	if s.sources.Memory != nil {
		mem = s.sources.Memory.Memory()
	}
	var temp any
	if s.sources.Temperature != nil {
		temp = s.sources.Temperature.Temperature()
	}
	var power any
	if s.sources.Power != nil {
		power = s.sources.Power.Power()
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"info":        info,
		"cpu":         cpu,
		"memory":      mem,
		"temperature": temp,
		"power":       power,
		"demo":        s.cfg.Demo,
	})
}

func (s *Server) handleCPUSnapshot(w http.ResponseWriter, r *http.Request) {
	if s.sources.CPU == nil {
		s.writeErr(w, http.StatusNotFound, "cpu source unavailable")
		return
	}
	s.writeJSON(w, http.StatusOK, s.sources.CPU.CPU())
}

func (s *Server) handleMemorySnapshot(w http.ResponseWriter, r *http.Request) {
	if s.sources.Memory == nil {
		s.writeErr(w, http.StatusNotFound, "memory source unavailable")
		return
	}
	s.writeJSON(w, http.StatusOK, s.sources.Memory.Memory())
}

func (s *Server) handleTemperatureSnapshot(w http.ResponseWriter, r *http.Request) {
	if s.sources.Temperature == nil {
		s.writeErr(w, http.StatusNotFound, "temperature source unavailable")
		return
	}
	s.writeJSON(w, http.StatusOK, s.sources.Temperature.Temperature())
}

func (s *Server) handleNetworkInterfaces(w http.ResponseWriter, r *http.Request) {
	if s.sources.Network == nil {
		s.writeErr(w, http.StatusNotFound, "network source unavailable")
		return
	}
	ifaces, gw := s.sources.Network.Network()
	s.writeJSON(w, http.StatusOK, map[string]any{"interfaces": ifaces, "gateway": gw})
}

func (s *Server) handleNetworkDevices(w http.ResponseWriter, r *http.Request) {
	if s.sources.Devices == nil {
		s.writeErr(w, http.StatusNotFound, "device source unavailable")
		return
	}
	s.writeJSON(w, http.StatusOK, s.sources.Devices.Devices())
}

func (s *Server) handlePower(w http.ResponseWriter, r *http.Request) {
	if s.sources.Power == nil {
		s.writeErr(w, http.StatusNotFound, "power source unavailable")
		return
	}
	s.writeJSON(w, http.StatusOK, s.sources.Power.Power())
}
