package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

var (
	ErrRuntimeExtensionIDRequired                   = errors.New("runtime extension id is required")
	ErrRuntimeExtensionDuplicate                    = errors.New("runtime extension already registered")
	ErrRuntimeExtensionToolRequired                 = errors.New("runtime extension tool is required")
	ErrRuntimeExtensionToolNameRequired             = errors.New("runtime extension tool name is required")
	ErrRuntimeExtensionToolDuplicate                = errors.New("runtime extension tool already registered")
	ErrRuntimeExtensionToolEffectMissing            = errors.New("runtime extension tool effect is required")
	ErrRuntimeExtensionToolEffectInvalid            = errors.New("runtime extension tool effect is invalid")
	ErrRuntimeExtensionHookIDRequired               = errors.New("runtime extension precompute hook id is required")
	ErrRuntimeExtensionHookDuplicate                = errors.New("runtime extension precompute hook already registered")
	ErrRuntimeExtensionHookRequired                 = errors.New("runtime extension precompute hook is required")
	ErrRuntimeExtensionContextIDRequired            = errors.New("runtime extension context contribution id is required")
	ErrRuntimeExtensionContextDuplicate             = errors.New("runtime extension context contribution already registered")
	ErrRuntimeExtensionProviderMiddlewareIDRequired = errors.New("runtime extension provider middleware id is required")
	ErrRuntimeExtensionProviderMiddlewareDuplicate  = errors.New("runtime extension provider middleware already registered")
	ErrRuntimeExtensionProviderMiddlewareRequired   = errors.New("runtime extension provider middleware is required")
)

type RuntimeExtensionRegistration struct {
	ID                   string
	Tools                []RuntimeExtensionToolRegistration
	PrecomputeHooks      []RuntimeExtensionPrecomputeHookRegistration
	ContextContributions []RuntimeExtensionContextContribution
	ProviderMiddleware   []RuntimeExtensionProviderMiddlewareRegistration
}

type RuntimeExtensionToolRegistration struct {
	Tool    tool.Tool
	Effects []tool.ExecutionEffect
}

type RuntimeExtensionPrecomputeHookRegistration struct {
	ID   string
	Hook RuntimePrecomputeHook
}

type RuntimeExtensionContextContribution struct {
	ID          string
	Description string
}

type RuntimeExtensionProviderMiddlewareRegistration struct {
	ID         string
	Middleware provider.Middleware
}

type runtimeExtensionSurface struct {
	Tools                []tool.Tool
	ToolEffects          map[string][]tool.ExecutionEffect
	PrecomputeHooks      []RuntimePrecomputeHook
	ContextContributions []RuntimeExtensionContextContribution
	ProviderMiddleware   []provider.Middleware
}

var (
	runtimeExtensionsMu sync.RWMutex
	runtimeExtensions   = map[string]RuntimeExtensionRegistration{}
)

func RegisterRuntimeExtension(registration RuntimeExtensionRegistration) {
	if err := validateRuntimeExtensionRegistration(registration); err != nil {
		panic(err.Error())
	}
	id := strings.TrimSpace(registration.ID)

	runtimeExtensionsMu.Lock()
	defer runtimeExtensionsMu.Unlock()
	if _, exists := runtimeExtensions[id]; exists {
		panic(fmt.Sprintf("%v: %s", ErrRuntimeExtensionDuplicate, id))
	}
	registration.ID = id
	runtimeExtensions[id] = cloneRuntimeExtensionRegistration(registration)
}

func registeredRuntimeExtensions() []RuntimeExtensionRegistration {
	runtimeExtensionsMu.RLock()
	defer runtimeExtensionsMu.RUnlock()
	ids := make([]string, 0, len(runtimeExtensions))
	for id := range runtimeExtensions {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]RuntimeExtensionRegistration, 0, len(ids))
	for _, id := range ids {
		out = append(out, cloneRuntimeExtensionRegistration(runtimeExtensions[id]))
	}
	return out
}

func buildRuntimeExtensionSurface(registrations []RuntimeExtensionRegistration) (runtimeExtensionSurface, error) {
	surface := runtimeExtensionSurface{
		ToolEffects: map[string][]tool.ExecutionEffect{},
	}
	hookIDs := map[string]struct{}{}
	contextIDs := map[string]struct{}{}
	providerMiddlewareIDs := map[string]struct{}{}
	for _, registration := range registrations {
		if err := validateRuntimeExtensionRegistration(registration); err != nil {
			return runtimeExtensionSurface{}, err
		}
		for _, toolRegistration := range registration.Tools {
			name := strings.TrimSpace(toolRegistration.Tool.Definition().Name)
			if _, exists := surface.ToolEffects[name]; exists {
				return runtimeExtensionSurface{}, fmt.Errorf("%w: %s", ErrRuntimeExtensionToolDuplicate, name)
			}
			surface.Tools = append(surface.Tools, toolRegistration.Tool)
			surface.ToolEffects[name] = append([]tool.ExecutionEffect(nil), toolRegistration.Effects...)
		}
		for _, hookRegistration := range registration.PrecomputeHooks {
			id := strings.TrimSpace(hookRegistration.ID)
			if _, exists := hookIDs[id]; exists {
				return runtimeExtensionSurface{}, fmt.Errorf("%w: %s", ErrRuntimeExtensionHookDuplicate, id)
			}
			hookIDs[id] = struct{}{}
			surface.PrecomputeHooks = append(surface.PrecomputeHooks, hookRegistration.Hook)
		}
		for _, contribution := range registration.ContextContributions {
			id := strings.TrimSpace(contribution.ID)
			if _, exists := contextIDs[id]; exists {
				return runtimeExtensionSurface{}, fmt.Errorf("%w: %s", ErrRuntimeExtensionContextDuplicate, id)
			}
			contextIDs[id] = struct{}{}
			surface.ContextContributions = append(surface.ContextContributions, contribution)
		}
		for _, middlewareRegistration := range registration.ProviderMiddleware {
			id := strings.TrimSpace(middlewareRegistration.ID)
			if _, exists := providerMiddlewareIDs[id]; exists {
				return runtimeExtensionSurface{}, fmt.Errorf("%w: %s", ErrRuntimeExtensionProviderMiddlewareDuplicate, id)
			}
			providerMiddlewareIDs[id] = struct{}{}
			surface.ProviderMiddleware = append(surface.ProviderMiddleware, middlewareRegistration.Middleware)
		}
	}
	if len(surface.ToolEffects) == 0 {
		surface.ToolEffects = nil
	}
	return surface, nil
}

func validateRuntimeExtensionRegistration(registration RuntimeExtensionRegistration) error {
	if strings.TrimSpace(registration.ID) == "" {
		return ErrRuntimeExtensionIDRequired
	}
	for _, toolRegistration := range registration.Tools {
		if toolRegistration.Tool == nil {
			return fmt.Errorf("%w: %s", ErrRuntimeExtensionToolRequired, registration.ID)
		}
		name := strings.TrimSpace(toolRegistration.Tool.Definition().Name)
		if name == "" {
			return fmt.Errorf("%w: %s", ErrRuntimeExtensionToolNameRequired, registration.ID)
		}
		if err := validateRuntimeExtensionToolEffects(name, toolRegistration.Effects); err != nil {
			return err
		}
	}
	for _, hookRegistration := range registration.PrecomputeHooks {
		if strings.TrimSpace(hookRegistration.ID) == "" {
			return fmt.Errorf("%w: %s", ErrRuntimeExtensionHookIDRequired, registration.ID)
		}
		if hookRegistration.Hook == nil {
			return fmt.Errorf("%w: %s", ErrRuntimeExtensionHookRequired, hookRegistration.ID)
		}
	}
	for _, contribution := range registration.ContextContributions {
		if strings.TrimSpace(contribution.ID) == "" {
			return fmt.Errorf("%w: %s", ErrRuntimeExtensionContextIDRequired, registration.ID)
		}
	}
	for _, middlewareRegistration := range registration.ProviderMiddleware {
		if strings.TrimSpace(middlewareRegistration.ID) == "" {
			return fmt.Errorf("%w: %s", ErrRuntimeExtensionProviderMiddlewareIDRequired, registration.ID)
		}
		if middlewareRegistration.Middleware == nil {
			return fmt.Errorf("%w: %s", ErrRuntimeExtensionProviderMiddlewareRequired, middlewareRegistration.ID)
		}
	}
	return nil
}

func validateRuntimeExtensionToolEffects(toolName string, effects []tool.ExecutionEffect) error {
	if len(effects) == 0 {
		return fmt.Errorf("%w: %s", ErrRuntimeExtensionToolEffectMissing, toolName)
	}
	for _, effect := range effects {
		switch effect {
		case tool.ExecutionEffectRead, tool.ExecutionEffectWrite, tool.ExecutionEffectNetwork, tool.ExecutionEffectProcess:
		default:
			return fmt.Errorf("%w: %s", ErrRuntimeExtensionToolEffectInvalid, toolName)
		}
	}
	return nil
}

func cloneRuntimeExtensionRegistration(registration RuntimeExtensionRegistration) RuntimeExtensionRegistration {
	registration.Tools = append([]RuntimeExtensionToolRegistration(nil), registration.Tools...)
	for idx := range registration.Tools {
		registration.Tools[idx].Effects = append([]tool.ExecutionEffect(nil), registration.Tools[idx].Effects...)
	}
	registration.PrecomputeHooks = append([]RuntimeExtensionPrecomputeHookRegistration(nil), registration.PrecomputeHooks...)
	registration.ContextContributions = append([]RuntimeExtensionContextContribution(nil), registration.ContextContributions...)
	registration.ProviderMiddleware = append([]RuntimeExtensionProviderMiddlewareRegistration(nil), registration.ProviderMiddleware...)
	return registration
}
