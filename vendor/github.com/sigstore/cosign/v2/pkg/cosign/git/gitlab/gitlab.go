//
// Copyright 2021 The Sigstore Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gitlab

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/sigstore/cosign/v2/internal/ui"
	"github.com/sigstore/cosign/v2/pkg/cosign"
	"github.com/sigstore/cosign/v2/pkg/cosign/env"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

const (
	ReferenceScheme = "gitlab"
)

type Gl struct{}

func New() *Gl {
	return &Gl{}
}

func (g *Gl) PutSecret(ctx context.Context, ref string, pf cosign.PassFunc) error {
	keys, err := cosign.GenerateKeyPair(pf)
	if err != nil {
		return fmt.Errorf("generating key pair: %w", err)
	}

	token, tokenExists := env.LookupEnv(env.VariableGitLabToken)

	if !tokenExists {
		return fmt.Errorf("could not find %q", env.VariableGitLabToken.String())
	}

	var client *gitlab.Client
	if url, baseURLExists := env.LookupEnv(env.VariableGitLabHost); baseURLExists {
		client, err = gitlab.NewClient(token, gitlab.WithBaseURL(url))
		if err != nil {
			return fmt.Errorf("could not create GitLab client: %w", err)
		}
	} else {
		client, err = gitlab.NewClient(token)
		if err != nil {
			return fmt.Errorf("could not create GitLab client: %w", err)
		}
	}

	_, passwordResp, err := client.ProjectVariables.CreateVariable(ref, &gitlab.CreateProjectVariableOptions{
		Key:              gitlab.Ptr("COSIGN_PASSWORD"),
		Value:            gitlab.Ptr(string(keys.Password())),
		VariableType:     gitlab.Ptr(gitlab.EnvVariableType),
		Protected:        gitlab.Ptr(false),
		Masked:           gitlab.Ptr(false),
		EnvironmentScope: gitlab.Ptr("*"),
	})
	if err != nil {
		ui.Warnf(ctx, "If you are using a self-hosted gitlab please set the \"GITLAB_HOST\" your server name.")
		return fmt.Errorf("could not create \"COSIGN_PASSWORD\" variable: %w", err)
	}

	if passwordResp.StatusCode < 200 && passwordResp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(passwordResp.Body)
		return fmt.Errorf("%s", bodyBytes)
	}

	ui.Infof(ctx, "Password written to \"COSIGN_PASSWORD\" variable")

	_, privateKeyResp, err := client.ProjectVariables.CreateVariable(ref, &gitlab.CreateProjectVariableOptions{
		Key:          gitlab.Ptr("COSIGN_PRIVATE_KEY"),
		Value:        gitlab.Ptr(string(keys.PrivateBytes)),
		VariableType: gitlab.Ptr(gitlab.EnvVariableType),
		Protected:    gitlab.Ptr(false),
		Masked:       gitlab.Ptr(false),
	})
	if err != nil {
		return fmt.Errorf("could not create \"COSIGN_PRIVATE_KEY\" variable: %w", err)
	}

	if privateKeyResp.StatusCode < 200 && privateKeyResp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(privateKeyResp.Body)
		return fmt.Errorf("%s", bodyBytes)
	}

	ui.Infof(ctx, "Private key written to \"COSIGN_PRIVATE_KEY\" variable")

	_, publicKeyResp, err := client.ProjectVariables.CreateVariable(ref, &gitlab.CreateProjectVariableOptions{
		Key:          gitlab.Ptr("COSIGN_PUBLIC_KEY"),
		Value:        gitlab.Ptr(string(keys.PublicBytes)),
		VariableType: gitlab.Ptr(gitlab.EnvVariableType),
		Protected:    gitlab.Ptr(false),
		Masked:       gitlab.Ptr(false),
	})
	if err != nil {
		return fmt.Errorf("could not create \"COSIGN_PUBLIC_KEY\" variable: %w", err)
	}

	if publicKeyResp.StatusCode < 200 && publicKeyResp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(publicKeyResp.Body)
		return fmt.Errorf("%s", bodyBytes)
	}

	ui.Infof(ctx, "Public key written to \"COSIGN_PUBLIC_KEY\" variable")

	if err := os.WriteFile("cosign.pub", keys.PublicBytes, 0o600); err != nil {
		return err
	}
	ui.Infof(ctx, "Public key also written to cosign.pub")

	return nil
}

func (g *Gl) GetSecret(_ context.Context, ref string, key string) (string, error) {
	token, tokenExists := env.LookupEnv(env.VariableGitLabToken)
	var varPubKeyValue string
	if !tokenExists {
		return varPubKeyValue, fmt.Errorf("could not find %q", env.VariableGitLabToken.String())
	}

	var client *gitlab.Client
	var err error
	if url, baseURLExists := env.LookupEnv(env.VariableGitLabHost); baseURLExists {
		client, err = gitlab.NewClient(token, gitlab.WithBaseURL(url))
		if err != nil {
			return varPubKeyValue, fmt.Errorf("could not create GitLab client): %w", err)
		}
	} else {
		client, err = gitlab.NewClient(token)
		if err != nil {
			return varPubKeyValue, fmt.Errorf("could not create GitLab client: %w", err)
		}
	}

	varPubKey, pubKeyResp, err := client.ProjectVariables.GetVariable(ref, key, nil)
	if err != nil {
		return varPubKeyValue, fmt.Errorf("could not retrieve \"COSIGN_PUBLIC_KEY\" variable: %w", err)
	}

	varPubKeyValue = varPubKey.Value

	if pubKeyResp.StatusCode < 200 && pubKeyResp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(pubKeyResp.Body)
		return varPubKeyValue, fmt.Errorf("%s", bodyBytes)
	}

	return varPubKeyValue, nil
}
