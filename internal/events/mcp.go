package events

import "errors"

type SessionMCPCatalogUpdatedPayload struct {
	WorkspaceTrusted bool
	Servers          []SessionMCPServerPayload
	Tools            []SessionMCPToolPayload
}

func (SessionMCPCatalogUpdatedPayload) eventType() Type { return TypeSessionMCPCatalogUpdated }

func (p SessionMCPCatalogUpdatedPayload) validate() error {
	for _, server := range p.Servers {
		if err := server.validate(); err != nil {
			return err
		}
	}
	for _, tl := range p.Tools {
		if err := tl.validate(); err != nil {
			return err
		}
	}
	return nil
}

type SessionMCPServerPayload struct {
	Name        string
	Type        string
	Fingerprint string
	Trusted     bool
	Active      bool
}

func (p SessionMCPServerPayload) validate() error {
	if p.Name == "" {
		return errors.New("server name is required")
	}
	if p.Type == "" {
		return errors.New("server type is required")
	}
	if p.Fingerprint == "" {
		return errors.New("server fingerprint is required")
	}
	return nil
}

type SessionMCPToolPayload struct {
	Name        string
	Description string
	InputSchema string
	ServerName  string
	RemoteName  string
}

func (p SessionMCPToolPayload) validate() error {
	if p.Name == "" {
		return errors.New("tool name is required")
	}
	return nil
}

type SessionMCPState struct {
	WorkspaceTrusted bool
	Servers          []SessionMCPServerPayload
	Tools            []SessionMCPToolPayload
}
