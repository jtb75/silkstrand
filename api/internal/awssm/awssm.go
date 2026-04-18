// Package awssm provides an AWS Secrets Manager credential resolver
// for ADR 004 C1. The DC API calls Resolve at scan time to fetch
// credentials server-side; the agent receives plaintext the same way
// it does for static sources.
package awssm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// ResolveConfig holds the per-credential-source configuration stored
// in credential_sources.config JSONB for type=aws_secrets_manager.
type ResolveConfig struct {
	Region           string `json:"region"`
	SecretARN        string `json:"secret_arn"`
	RoleARN          string `json:"role_arn,omitempty"`            // optional; assume role if set
	SecretKeyUsername string `json:"secret_key_username,omitempty"` // JSON key for username in the secret value
	SecretKeyPassword string `json:"secret_key_password,omitempty"` // JSON key for password
}

// Credential is the resolved username + password extracted from the
// AWS Secrets Manager secret value.
type Credential struct {
	Username string
	Password string
}

// Resolve fetches a secret from AWS Secrets Manager and extracts
// username + password from the JSON secret value.
//
// The flow:
//  1. Load AWS config for the specified region.
//  2. If RoleARN is set, assume that role via STS (cross-account).
//  3. Call GetSecretValue with the configured SecretARN.
//  4. Parse SecretString as JSON.
//  5. Extract username + password using the configured key names.
func Resolve(ctx context.Context, cfg ResolveConfig) (*Credential, error) {
	if cfg.Region == "" {
		return nil, fmt.Errorf("awssm: region is required")
	}
	if cfg.SecretARN == "" {
		return nil, fmt.Errorf("awssm: secret_arn is required")
	}

	// Default key names if not specified.
	usernameKey := cfg.SecretKeyUsername
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := cfg.SecretKeyPassword
	if passwordKey == "" {
		passwordKey = "password"
	}

	// 1. Load AWS config for the specified region.
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("awssm: loading AWS config: %w", err)
	}

	// 2. If RoleARN is set, assume role via STS.
	if cfg.RoleARN != "" {
		stsClient := sts.NewFromConfig(awsCfg)
		creds := stscreds.NewAssumeRoleProvider(stsClient, cfg.RoleARN)
		awsCfg.Credentials = aws.NewCredentialsCache(creds)
	}

	// 3. Call GetSecretValue.
	smClient := secretsmanager.NewFromConfig(awsCfg)
	result, err := smClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(cfg.SecretARN),
	})
	if err != nil {
		return nil, fmt.Errorf("awssm: GetSecretValue: %w", err)
	}

	if result.SecretString == nil {
		return nil, fmt.Errorf("awssm: secret %s has no SecretString (binary secrets not supported)", cfg.SecretARN)
	}

	// 4. Parse the SecretString as JSON.
	var secretData map[string]any
	if err := json.Unmarshal([]byte(*result.SecretString), &secretData); err != nil {
		return nil, fmt.Errorf("awssm: parsing SecretString as JSON: %w", err)
	}

	// 5. Extract username + password using the configured keys.
	username, ok := secretData[usernameKey].(string)
	if !ok {
		return nil, fmt.Errorf("awssm: key %q not found or not a string in secret", usernameKey)
	}
	password, ok := secretData[passwordKey].(string)
	if !ok {
		return nil, fmt.Errorf("awssm: key %q not found or not a string in secret", passwordKey)
	}

	return &Credential{
		Username: username,
		Password: password,
	}, nil
}
