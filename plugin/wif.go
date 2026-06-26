package main

import (
	"context"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google/externalaccount"
)

const (
	googleIAMCredentialsPrefix = "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/"
	googleIAMCredentialsSuffix = ":generateAccessToken"
	googleCloudScope           = "https://www.googleapis.com/auth/cloud-platform"
	awsSubjectTokenType        = "urn:ietf:params:aws:token-type:aws4_request"
	googleWIFAudiencePrefix    = "//iam.googleapis.com/projects/"
)

type wifConfig struct {
	AWSRegion            string
	ProjectNumber        string
	PoolID               string
	ProviderID           string
	GoogleServiceAccount string
}

type awsSDKCredentialSupplier struct {
	awsConfig aws.Config
	region    string
}

func (c wifConfig) missingRequiredOptions() []string {
	required := []struct {
		name  string
		value string
	}{
		{optionAWSRegion, c.AWSRegion},
		{optionProjectNumber, c.ProjectNumber},
		{optionPoolID, c.PoolID},
		{optionProviderID, c.ProviderID},
		{optionGoogleServiceAccount, c.GoogleServiceAccount},
	}

	missing := []string{}
	for _, option := range required {
		if option.value == "" {
			missing = append(missing, option.name)
		}
	}
	return missing
}

func newWIFTokenSource(ctx context.Context, cfg wifConfig) (oauth2.TokenSource, error) {
	if missing := cfg.missingRequiredOptions(); len(missing) > 0 {
		return nil, fmt.Errorf("missing required WIF option(s): %v", missing)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	targetResource := googleWIFAudiencePrefix + cfg.ProjectNumber + "/locations/global/workloadIdentityPools/" + cfg.PoolID + "/providers/" + cfg.ProviderID

	return externalaccount.NewTokenSource(ctx, externalaccount.Config{
		Audience:                       targetResource,
		SubjectTokenType:               awsSubjectTokenType,
		Scopes:                         []string{googleCloudScope},
		ServiceAccountImpersonationURL: googleIAMCredentialsPrefix + url.PathEscape(cfg.GoogleServiceAccount) + googleIAMCredentialsSuffix,
		AwsSecurityCredentialsSupplier: &awsSDKCredentialSupplier{
			awsConfig: awsCfg,
			region:    cfg.AWSRegion,
		},
	})
}

func (s *awsSDKCredentialSupplier) AwsRegion(ctx context.Context, _ externalaccount.SupplierOptions) (string, error) {
	return s.region, nil
}

func (s *awsSDKCredentialSupplier) AwsSecurityCredentials(ctx context.Context, _ externalaccount.SupplierOptions) (*externalaccount.AwsSecurityCredentials, error) {
	creds, err := s.awsConfig.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("retrieve AWS credentials: %w", err)
	}

	return &externalaccount.AwsSecurityCredentials{
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
	}, nil
}
