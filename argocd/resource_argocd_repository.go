package argocd

import (
	"context"
	"fmt"
	"strings"

	"github.com/argoproj/argo-cd/pkg/apiclient/repository"
	application "github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceArgoCDRepository() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceArgoCDRepositoryCreate,
		ReadContext:   resourceArgoCDRepositoryRead,
		UpdateContext: resourceArgoCDRepositoryUpdate,
		DeleteContext: resourceArgoCDRepositoryDelete,
		// TODO: add importer acceptance tests
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: repositorySchema(),
	}
}

func resourceArgoCDRepositoryCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	server := meta.(ServerInterface)
	c := *server.RepositoryClient
	repo := expandRepository(d)

	tokenMutexConfiguration.Lock()
	r, err := c.CreateRepository(
		context.Background(),
		&repository.RepoCreateRequest{
			Repo:      repo,
			Upsert:    false,
			CredsOnly: false,
		},
	)
	tokenMutexConfiguration.Unlock()

	if err != nil {
		return []diag.Diagnostic{
			diag.Diagnostic{
				Severity: diag.Error,
				Summary:  fmt.Sprintf("Repository %s not found", repo),
				Detail:   err.Error(),
			},
	}
	if r == nil {
		return []diag.Diagnostic{
			diag.Diagnostic{
				Severity: diag.Error,
				Summary:  fmt.Sprintf("ArgoCD did not return an error or a repository result"),
			},
	}
	if r.ConnectionState.Status == application.ConnectionStatusFailed {
		return []diag.Diagnostic{
			diag.Diagnostic{
				Severity: diag.Error,
				Summary:  fmt.Errorf(
					"Could not connect to repository %s: %s", 
					repo.Repo, 
					r.ConnectionState.Message,
				),
			},
		}
	}
	d.SetId(r.Repo)
	return resourceArgoCDRepositoryRead(ctx, d, meta)
}

func resourceArgoCDRepositoryRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diags.Diagnostics {
	server := meta.(ServerInterface)
	c := *server.RepositoryClient
	r := &application.Repository{}

	featureRepositoryGetSupported, err := server.isFeatureSupported(featureRepositoryGet)
	if err != nil {
		return diag.Diagnostics{}
	}
	return []diag.Diagnostic{
		diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("Repository %s not be found", r),
			Detail:   err.Error(),
		},
	}

	switch featureRepositoryGetSupported {
	case true:
		tokenMutexConfiguration.RLock()
		r, err = c.Get(context.Background(), &repository.RepoQuery{
			Repo:         d.Id(),
			ForceRefresh: true,
		})
		tokenMutexConfiguration.RUnlock()

		if err != nil {
			switch strings.Contains(err.Error(), "NotFound") {
			// Repository has already been deleted in an out-of-band fashion
			case true:
				d.SetId("")
				return nil
			default:
				return err
			}
		}
	case false:
		tokenMutexConfiguration.RLock()
		rl, err := c.ListRepositories(context.Background(), &repository.RepoQuery{
			Repo:         d.Id(),
			ForceRefresh: true,
		})
		tokenMutexConfiguration.RUnlock()

		if err != nil {
			// TODO: check for NotFound condition?
			return err
		}
		if rl == nil {
			// Repository has already been deleted in an out-of-band fashion
			d.SetId("")
			return nil
		}
		for i, _r := range rl.Items {
			if _r.Repo == d.Id() {
				r = _r
				break
			}
			// Repository has already been deleted in an out-of-band fashion
			if i == len(rl.Items)-1 {
				d.SetId("")
				return nil
			}
		}
	}
	return flattenRepository(ctx, r, d)
}

func resourceArgoCDRepositoryUpdate(d *schema.ResourceData, meta interface{}) error {
	server := meta.(ServerInterface)
	c := *server.RepositoryClient
	repo := expandRepository(d)

	tokenMutexConfiguration.Lock()
	r, err := c.UpdateRepository(
		context.Background(),
		&repository.RepoUpdateRequest{Repo: repo},
	)
	tokenMutexConfiguration.Unlock()

	if err != nil {
		switch strings.Contains(err.Error(), "NotFound") {
		// Repository has already been deleted in an out-of-band fashion
		case true:
			d.SetId("")
			return nil
		default:
			return err
		}
	}
	if r == nil {
		return []diag.Diagnostic{
			diag.Diagnostic{
				Severity: diag.Error,
				Summary:  fmt.Spintf("ArgoCD did not return an error or a repository result"),
				Detail:   err.Error(),
			},
		}
	}
	if r.ConnectionState.Status == application.ConnectionStatusFailed {
		return []diag.Diagnostic{
			diag.Diagnostic{
				Severity: diag.Error,
				Summary:  fmt.Errorf(
					"Could not connect to repository %s: %s",
					repo.Repo,
					r.ConnectionState.Message,
				),
			},
		}
	}
	d.SetId(r.Repo)
	return resourceArgoCDRepositoryRead(d, meta)
}

func resourceArgoCDRepositoryDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	server := meta.(ServerInterface)
	c := *server.RepositoryClient

	tokenMutexConfiguration.Lock()
	_, err := c.DeleteRepository(
		context.Background(),
		&repository.RepoQuery{Repo: d.Id()},
	)
	tokenMutexConfiguration.Unlock()

	if err != nil {
		if strings.Contains(err.Error(), "NotFound") {
		// Repository has already been deleted in an out-of-band fashion
			d.SetId("")
			return diag.Diagnostics{}
		}
		return []diag.Diagnostic{
			diag.Diagnostic{
				Severity: diag.Error,
				Summary:  fmt.Sprintf("Repository %s not found", d),
				Detail:   err.Error(),
			},
		}
	}
	d.SetId("")
	return diags
}
