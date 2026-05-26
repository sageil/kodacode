package tui

import (
	"context"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
)

func (b *LocalBackend) SetPrimaryModel(ctx context.Context, sessionID string, model provider.ModelRef) error {
	if strings.TrimSpace(sessionID) == "" {
		return app.ErrSessionIDRequired
	}
	route := provider.ModelRoute{Primary: model}
	if err := b.runtime.ValidateModelRoute(route); err != nil {
		return err
	}
	if err := b.applyReconfigurableMutation(func(configStore *app.ConfigStore, _ *provider.AuthStore) error {
		return configStore.SetModelRoute(route.Primary.String())
	}); err != nil {
		return err
	}
	if err := b.runtime.SetSessionModelRoute(ctx, sessionID, route); err != nil {
		return err
	}
	return nil
}

func (b *LocalBackend) SetUtilityModel(_ context.Context, model provider.ModelRef) error {
	if strings.TrimSpace(model.ProviderID) != "" || strings.TrimSpace(model.ModelID) != "" {
		if err := model.Validate(); err != nil {
			return err
		}
	}
	return b.applyReconfigurableMutation(func(configStore *app.ConfigStore, _ *provider.AuthStore) error {
		return configStore.SetUtilityModel(optionalModelRefString(model))
	})
}

func (b *LocalBackend) SetReviewerModel(_ context.Context, model provider.ModelRef) error {
	if strings.TrimSpace(model.ProviderID) != "" || strings.TrimSpace(model.ModelID) != "" {
		route := provider.ModelRoute{Primary: model}
		if err := b.runtime.ValidateModelRoute(route); err != nil {
			return err
		}
	}
	return b.applyReconfigurableMutation(func(configStore *app.ConfigStore, _ *provider.AuthStore) error {
		return configStore.SetReviewModelRoute(optionalModelRefString(model))
	})
}

func (b *LocalBackend) RefreshModelCatalog(ctx context.Context) (app.DialogState, error) {
	if b == nil || b.runtime == nil {
		return app.DialogState{}, nil
	}
	refreshErr := b.runtime.RefreshModelCatalog(ctx)
	state, err := b.runtime.DialogState()
	if err != nil {
		return app.DialogState{}, errors.Join(refreshErr, err)
	}
	return state, app.UserFacingModelCatalogRefreshError(refreshErr)
}
