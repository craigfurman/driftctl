package remote

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/pkg/errors"
	"github.com/snyk/driftctl/mocks"
	"github.com/snyk/driftctl/pkg/filter"
	"github.com/snyk/driftctl/pkg/remote/alerts"
	remoteaws "github.com/snyk/driftctl/pkg/remote/aws"
	"github.com/snyk/driftctl/pkg/remote/aws/repository"
	"github.com/snyk/driftctl/pkg/remote/cache"
	"github.com/snyk/driftctl/pkg/remote/common"
	remoteerr "github.com/snyk/driftctl/pkg/remote/error"
	"github.com/snyk/driftctl/pkg/resource"
	resourceaws "github.com/snyk/driftctl/pkg/resource/aws"
	"github.com/snyk/driftctl/pkg/terraform"
	"github.com/snyk/driftctl/test"
	"github.com/snyk/driftctl/test/goldenfile"
	testresource "github.com/snyk/driftctl/test/resource"
	terraform2 "github.com/snyk/driftctl/test/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestIamUser(t *testing.T) {

	cases := []struct {
		test    string
		dirName string
		mocks   func(*repository.MockIAMRepository, *mocks.AlerterInterface)
		wantErr error
	}{
		{
			test:    "no iam user",
			dirName: "aws_iam_user_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				repo.On("ListAllUsers").Return([]*iam.User{}, nil)
			},
			wantErr: nil,
		},
		{
			test:    "iam multiples users",
			dirName: "aws_iam_user_multiple",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				repo.On("ListAllUsers").Return([]*iam.User{
					{
						UserName: aws.String("test-driftctl-0"),
					},
					{
						UserName: aws.String("test-driftctl-1"),
					},
					{
						UserName: aws.String("test-driftctl-2"),
					},
				}, nil)
			},
			wantErr: nil,
		},
		{
			test:    "cannot list iam user",
			dirName: "aws_iam_user_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				awsError := awserr.NewRequestFailure(awserr.New("AccessDeniedException", "", errors.New("")), 403, "")
				repo.On("ListAllUsers").Return(nil, awsError)

				alerter.On("SendAlert", resourceaws.AwsIamUserResourceType, alerts.NewRemoteAccessDeniedAlert(common.RemoteAWSTerraform, remoteerr.NewResourceListingErrorWithType(awsError, resourceaws.AwsIamUserResourceType, resourceaws.AwsIamUserResourceType), alerts.EnumerationPhase)).Return()
			},
			wantErr: nil,
		},
	}
	schemaRepository := testresource.InitFakeSchemaRepository("aws", "3.19.0")
	resourceaws.InitResourcesMetadata(schemaRepository)
	factory := terraform.NewTerraformResourceFactory(schemaRepository)
	deserializer := resource.NewDeserializer(factory)

	for _, c := range cases {
		t.Run(c.test, func(tt *testing.T) {
			shouldUpdate := c.dirName == *goldenfile.Update

			sess := session.Must(session.NewSessionWithOptions(session.Options{
				SharedConfigState: session.SharedConfigEnable,
			}))

			scanOptions := ScannerOptions{Deep: true}
			providerLibrary := terraform.NewProviderLibrary()
			remoteLibrary := common.NewRemoteLibrary()

			// Initialize mocks
			alerter := &mocks.AlerterInterface{}
			fakeRepo := &repository.MockIAMRepository{}
			c.mocks(fakeRepo, alerter)

			var repo repository.IAMRepository = fakeRepo
			providerVersion := "3.19.0"
			realProvider, err := terraform2.InitTestAwsProvider(providerLibrary, providerVersion)
			if err != nil {
				t.Fatal(err)
			}
			provider := terraform2.NewFakeTerraformProvider(realProvider)
			provider.WithResponse(c.dirName)

			// Replace mock by real resources if we are in update mode
			if shouldUpdate {
				err := realProvider.Init()
				if err != nil {
					t.Fatal(err)
				}
				provider.ShouldUpdate()
				repo = repository.NewIAMRepository(sess, cache.New(0))
			}

			remoteLibrary.AddEnumerator(remoteaws.NewIamUserEnumerator(repo, factory))
			remoteLibrary.AddDetailsFetcher(resourceaws.AwsIamUserResourceType, common.NewGenericDetailsFetcher(resourceaws.AwsIamUserResourceType, provider, deserializer))

			testFilter := &filter.MockFilter{}
			testFilter.On("IsTypeIgnored", mock.Anything).Return(false)

			s := NewScanner(remoteLibrary, alerter, scanOptions, testFilter)
			got, err := s.Resources()
			assert.Equal(tt, err, c.wantErr)
			if err != nil {
				return
			}
			test.TestAgainstGoldenFile(got, resourceaws.AwsIamUserResourceType, c.dirName, provider, deserializer, shouldUpdate, tt)
			alerter.AssertExpectations(tt)
			fakeRepo.AssertExpectations(tt)
		})
	}
}

func TestIamUserPolicy(t *testing.T) {

	cases := []struct {
		test    string
		dirName string
		mocks   func(*repository.MockIAMRepository, *mocks.AlerterInterface)
		wantErr error
	}{
		{
			test:    "no iam user policy",
			dirName: "aws_iam_user_policy_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				users := []*iam.User{
					{
						UserName: aws.String("loadbalancer"),
					},
				}
				repo.On("ListAllUsers").Return(users, nil)
				repo.On("ListAllUserPolicies", users).Return([]string{}, nil)
			},
			wantErr: nil,
		},
		{
			test:    "iam multiples users multiple policies",
			dirName: "aws_iam_user_policy_multiple",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				users := []*iam.User{
					{
						UserName: aws.String("loadbalancer"),
					},
					{
						UserName: aws.String("loadbalancer2"),
					},
					{
						UserName: aws.String("loadbalancer3"),
					},
				}
				repo.On("ListAllUsers").Return(users, nil)
				repo.On("ListAllUserPolicies", users).Once().Return([]string{
					*aws.String("loadbalancer:test"),
					*aws.String("loadbalancer:test2"),
					*aws.String("loadbalancer:test3"),
					*aws.String("loadbalancer:test4"),
					*aws.String("loadbalancer2:test2"),
					*aws.String("loadbalancer2:test22"),
					*aws.String("loadbalancer2:test23"),
					*aws.String("loadbalancer2:test24"),
					*aws.String("loadbalancer3:test3"),
					*aws.String("loadbalancer3:test32"),
					*aws.String("loadbalancer3:test33"),
					*aws.String("loadbalancer3:test34"),
				}, nil)
			},
			wantErr: nil,
		},
		{
			test:    "cannot list user",
			dirName: "aws_iam_user_policy_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				awsError := awserr.NewRequestFailure(awserr.New("AccessDeniedException", "", errors.New("")), 403, "")
				repo.On("ListAllUsers").Return(nil, awsError)

				alerter.On("SendAlert", resourceaws.AwsIamUserPolicyResourceType, alerts.NewRemoteAccessDeniedAlert(common.RemoteAWSTerraform, remoteerr.NewResourceListingErrorWithType(awsError, resourceaws.AwsIamUserPolicyResourceType, resourceaws.AwsIamUserResourceType), alerts.EnumerationPhase)).Return()
			},
			wantErr: nil,
		},
		{
			test:    "cannot list user policy",
			dirName: "aws_iam_user_policy_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				repo.On("ListAllUsers").Once().Return([]*iam.User{}, nil)
				awsError := awserr.NewRequestFailure(awserr.New("AccessDeniedException", "", errors.New("")), 403, "")
				repo.On("ListAllUserPolicies", mock.Anything).Return(nil, awsError)

				alerter.On("SendAlert", resourceaws.AwsIamUserPolicyResourceType, alerts.NewRemoteAccessDeniedAlert(common.RemoteAWSTerraform, remoteerr.NewResourceListingErrorWithType(awsError, resourceaws.AwsIamUserPolicyResourceType, resourceaws.AwsIamUserPolicyResourceType), alerts.EnumerationPhase)).Return()
			},
			wantErr: nil,
		},
	}
	schemaRepository := testresource.InitFakeSchemaRepository("aws", "3.19.0")
	resourceaws.InitResourcesMetadata(schemaRepository)
	factory := terraform.NewTerraformResourceFactory(schemaRepository)
	deserializer := resource.NewDeserializer(factory)

	for _, c := range cases {
		t.Run(c.test, func(tt *testing.T) {
			shouldUpdate := c.dirName == *goldenfile.Update

			sess := session.Must(session.NewSessionWithOptions(session.Options{
				SharedConfigState: session.SharedConfigEnable,
			}))

			scanOptions := ScannerOptions{Deep: true}
			providerLibrary := terraform.NewProviderLibrary()
			remoteLibrary := common.NewRemoteLibrary()

			// Initialize mocks
			alerter := &mocks.AlerterInterface{}
			fakeRepo := &repository.MockIAMRepository{}
			c.mocks(fakeRepo, alerter)

			var repo repository.IAMRepository = fakeRepo
			providerVersion := "3.19.0"
			realProvider, err := terraform2.InitTestAwsProvider(providerLibrary, providerVersion)
			if err != nil {
				t.Fatal(err)
			}
			provider := terraform2.NewFakeTerraformProvider(realProvider)
			provider.WithResponse(c.dirName)

			// Replace mock by real resources if we are in update mode
			if shouldUpdate {
				err := realProvider.Init()
				if err != nil {
					t.Fatal(err)
				}
				provider.ShouldUpdate()
				repo = repository.NewIAMRepository(sess, cache.New(0))
			}

			remoteLibrary.AddEnumerator(remoteaws.NewIamUserPolicyEnumerator(repo, factory))
			remoteLibrary.AddDetailsFetcher(resourceaws.AwsIamUserPolicyResourceType, common.NewGenericDetailsFetcher(resourceaws.AwsIamUserPolicyResourceType, provider, deserializer))

			testFilter := &filter.MockFilter{}
			testFilter.On("IsTypeIgnored", mock.Anything).Return(false)

			s := NewScanner(remoteLibrary, alerter, scanOptions, testFilter)
			got, err := s.Resources()
			assert.Equal(tt, err, c.wantErr)
			if err != nil {
				return
			}
			test.TestAgainstGoldenFile(got, resourceaws.AwsIamUserPolicyResourceType, c.dirName, provider, deserializer, shouldUpdate, tt)
			alerter.AssertExpectations(tt)
			fakeRepo.AssertExpectations(tt)
		})
	}
}

func TestIamPolicy(t *testing.T) {

	cases := []struct {
		test    string
		dirName string
		mocks   func(*repository.MockIAMRepository, *mocks.AlerterInterface)
		wantErr error
	}{
		{
			test:    "no iam custom policies",
			dirName: "aws_iam_policy_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				repo.On("ListAllPolicies").Once().Return([]*iam.Policy{}, nil)
			},
			wantErr: nil,
		},
		{
			test:    "iam multiples custom policies",
			dirName: "aws_iam_policy_multiple",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				repo.On("ListAllPolicies").Once().Return([]*iam.Policy{
					{
						Arn: aws.String("arn:aws:iam::929327065333:policy/policy-0"),
					},
					{
						Arn: aws.String("arn:aws:iam::929327065333:policy/policy-1"),
					},
					{
						Arn: aws.String("arn:aws:iam::929327065333:policy/policy-2"),
					},
				}, nil)
			},
			wantErr: nil,
		},
		{
			test:    "cannot list iam custom policies",
			dirName: "aws_iam_policy_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				awsError := awserr.NewRequestFailure(awserr.New("AccessDeniedException", "", errors.New("")), 403, "")
				repo.On("ListAllPolicies").Once().Return(nil, awsError)

				alerter.On("SendAlert", resourceaws.AwsIamPolicyResourceType, alerts.NewRemoteAccessDeniedAlert(common.RemoteAWSTerraform, remoteerr.NewResourceListingErrorWithType(awsError, resourceaws.AwsIamPolicyResourceType, resourceaws.AwsIamPolicyResourceType), alerts.EnumerationPhase)).Return()
			},
			wantErr: nil,
		},
	}

	schemaRepository := testresource.InitFakeSchemaRepository("aws", "3.19.0")
	resourceaws.InitResourcesMetadata(schemaRepository)
	factory := terraform.NewTerraformResourceFactory(schemaRepository)
	deserializer := resource.NewDeserializer(factory)

	for _, c := range cases {
		t.Run(c.test, func(tt *testing.T) {
			shouldUpdate := c.dirName == *goldenfile.Update

			sess := session.Must(session.NewSessionWithOptions(session.Options{
				SharedConfigState: session.SharedConfigEnable,
			}))

			scanOptions := ScannerOptions{Deep: true}
			providerLibrary := terraform.NewProviderLibrary()
			remoteLibrary := common.NewRemoteLibrary()

			// Initialize mocks
			alerter := &mocks.AlerterInterface{}
			fakeRepo := &repository.MockIAMRepository{}
			c.mocks(fakeRepo, alerter)

			var repo repository.IAMRepository = fakeRepo
			providerVersion := "3.19.0"
			realProvider, err := terraform2.InitTestAwsProvider(providerLibrary, providerVersion)
			if err != nil {
				t.Fatal(err)
			}
			provider := terraform2.NewFakeTerraformProvider(realProvider)
			provider.WithResponse(c.dirName)

			// Replace mock by real resources if we are in update mode
			if shouldUpdate {
				err := realProvider.Init()
				if err != nil {
					t.Fatal(err)
				}
				provider.ShouldUpdate()
				repo = repository.NewIAMRepository(sess, cache.New(0))
			}

			remoteLibrary.AddEnumerator(remoteaws.NewIamPolicyEnumerator(repo, factory))
			remoteLibrary.AddDetailsFetcher(resourceaws.AwsIamPolicyResourceType, common.NewGenericDetailsFetcher(resourceaws.AwsIamPolicyResourceType, provider, deserializer))

			testFilter := &filter.MockFilter{}
			testFilter.On("IsTypeIgnored", mock.Anything).Return(false)

			s := NewScanner(remoteLibrary, alerter, scanOptions, testFilter)
			got, err := s.Resources()
			assert.Equal(tt, err, c.wantErr)
			if err != nil {
				return
			}
			test.TestAgainstGoldenFile(got, resourceaws.AwsIamPolicyResourceType, c.dirName, provider, deserializer, shouldUpdate, tt)
			alerter.AssertExpectations(tt)
			fakeRepo.AssertExpectations(tt)
		})
	}
}

func TestIamRole(t *testing.T) {

	cases := []struct {
		test    string
		dirName string
		mocks   func(*repository.MockIAMRepository, *mocks.AlerterInterface)
		wantErr error
	}{
		{
			test:    "no iam roles",
			dirName: "aws_iam_role_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				repo.On("ListAllRoles").Return([]*iam.Role{}, nil)
			},
			wantErr: nil,
		},
		{
			test:    "iam multiples roles",
			dirName: "aws_iam_role_multiple",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				repo.On("ListAllRoles").Return([]*iam.Role{
					{
						RoleName: aws.String("test_role_0"),
						Path:     aws.String("/"),
					},
					{
						RoleName: aws.String("test_role_1"),
						Path:     aws.String("/"),
					},
					{
						RoleName: aws.String("test_role_2"),
						Path:     aws.String("/"),
					},
				}, nil)
			},
			wantErr: nil,
		},
		{
			test:    "iam roles ignore services roles",
			dirName: "aws_iam_role_ignore_services_roles",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				repo.On("ListAllRoles").Return([]*iam.Role{
					{
						RoleName: aws.String("AWSServiceRoleForOrganizations"),
						Path:     aws.String("/aws-service-role/organizations.amazonaws.com/"),
					},
					{
						RoleName: aws.String("AWSServiceRoleForSupport"),
						Path:     aws.String("/aws-service-role/support.amazonaws.com/"),
					},
					{
						RoleName: aws.String("AWSServiceRoleForTrustedAdvisor"),
						Path:     aws.String("/aws-service-role/trustedadvisor.amazonaws.com/"),
					},
				}, nil)
			},
			wantErr: nil,
		},
	}

	schemaRepository := testresource.InitFakeSchemaRepository("aws", "3.19.0")
	resourceaws.InitResourcesMetadata(schemaRepository)
	factory := terraform.NewTerraformResourceFactory(schemaRepository)
	deserializer := resource.NewDeserializer(factory)

	for _, c := range cases {
		t.Run(c.test, func(tt *testing.T) {
			shouldUpdate := c.dirName == *goldenfile.Update

			sess := session.Must(session.NewSessionWithOptions(session.Options{
				SharedConfigState: session.SharedConfigEnable,
			}))

			scanOptions := ScannerOptions{Deep: true}
			providerLibrary := terraform.NewProviderLibrary()
			remoteLibrary := common.NewRemoteLibrary()

			// Initialize mocks
			alerter := &mocks.AlerterInterface{}
			fakeRepo := &repository.MockIAMRepository{}
			c.mocks(fakeRepo, alerter)

			var repo repository.IAMRepository = fakeRepo
			providerVersion := "3.19.0"
			realProvider, err := terraform2.InitTestAwsProvider(providerLibrary, providerVersion)
			if err != nil {
				t.Fatal(err)
			}
			provider := terraform2.NewFakeTerraformProvider(realProvider)
			provider.WithResponse(c.dirName)

			// Replace mock by real resources if we are in update mode
			if shouldUpdate {
				err := realProvider.Init()
				if err != nil {
					t.Fatal(err)
				}
				provider.ShouldUpdate()
				repo = repository.NewIAMRepository(sess, cache.New(0))
			}

			remoteLibrary.AddEnumerator(remoteaws.NewIamRoleEnumerator(repo, factory))
			remoteLibrary.AddDetailsFetcher(resourceaws.AwsIamRoleResourceType, common.NewGenericDetailsFetcher(resourceaws.AwsIamRoleResourceType, provider, deserializer))

			testFilter := &filter.MockFilter{}
			testFilter.On("IsTypeIgnored", mock.Anything).Return(false)

			s := NewScanner(remoteLibrary, alerter, scanOptions, testFilter)
			got, err := s.Resources()
			assert.Equal(tt, err, c.wantErr)
			if err != nil {
				return
			}
			test.TestAgainstGoldenFile(got, resourceaws.AwsIamRoleResourceType, c.dirName, provider, deserializer, shouldUpdate, tt)
			alerter.AssertExpectations(tt)
			fakeRepo.AssertExpectations(tt)
		})
	}
}

func TestIamRolePolicyAttachment(t *testing.T) {

	cases := []struct {
		test    string
		dirName string
		mocks   func(*repository.MockIAMRepository, *mocks.AlerterInterface)
		err     error
	}{
		{
			test:    "no iam role policy",
			dirName: "aws_aws_iam_role_policy_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				roles := []*iam.Role{
					{
						RoleName: aws.String("test-role"),
					},
				}
				repo.On("ListAllRoles").Return(roles, nil)
				repo.On("ListAllRolePolicyAttachments", roles).Return([]*repository.AttachedRolePolicy{}, nil)
			},
			err: nil,
		},
		{
			test:    "iam multiples roles multiple policies",
			dirName: "aws_iam_role_policy_attachment_multiple",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				roles := []*iam.Role{
					{
						RoleName: aws.String("test-role"),
					},
					{
						RoleName: aws.String("test-role2"),
					},
				}
				repo.On("ListAllRoles").Return(roles, nil)
				repo.On("ListAllRolePolicyAttachments", roles).Return([]*repository.AttachedRolePolicy{
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::929327065333:policy/test-policy"),
							PolicyName: aws.String("test-policy"),
						},
						RoleName: *aws.String("test-role"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::929327065333:policy/test-policy2"),
							PolicyName: aws.String("test-policy2"),
						},
						RoleName: *aws.String("test-role"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::929327065333:policy/test-policy3"),
							PolicyName: aws.String("test-policy3"),
						},
						RoleName: *aws.String("test-role"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::929327065333:policy/test-policy"),
							PolicyName: aws.String("test-policy"),
						},
						RoleName: *aws.String("test-role2"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::929327065333:policy/test-policy2"),
							PolicyName: aws.String("test-policy2"),
						},
						RoleName: *aws.String("test-role2"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::929327065333:policy/test-policy3"),
							PolicyName: aws.String("test-policy3"),
						},
						RoleName: *aws.String("test-role2"),
					},
				}, nil)
			},
			err: nil,
		},
		{
			test:    "iam multiples roles for ignored roles",
			dirName: "aws_iam_role_policy_attachment_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				roles := []*iam.Role{
					{
						RoleName: aws.String("AWSServiceRoleForSupport"),
					},
					{
						RoleName: aws.String("AWSServiceRoleForOrganizations"),
					},
					{
						RoleName: aws.String("AWSServiceRoleForTrustedAdvisor"),
					},
				}
				repo.On("ListAllRoles").Return(roles, nil)
			},
		},
		{
			test:    "Cannot list roles",
			dirName: "aws_iam_role_policy_attachment_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				awsError := awserr.NewRequestFailure(awserr.New("AccessDeniedException", "", errors.New("")), 403, "")
				repo.On("ListAllRoles").Once().Return(nil, awsError)

				alerter.On("SendAlert", resourceaws.AwsIamRolePolicyAttachmentResourceType, alerts.NewRemoteAccessDeniedAlert(common.RemoteAWSTerraform, remoteerr.NewResourceListingErrorWithType(awsError, resourceaws.AwsIamRolePolicyAttachmentResourceType, resourceaws.AwsIamRoleResourceType), alerts.EnumerationPhase)).Return()
			},
		},
		{
			test:    "Cannot list roles policy attachment",
			dirName: "aws_iam_role_policy_attachment_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				repo.On("ListAllRoles").Once().Return([]*iam.Role{{RoleName: aws.String("test")}}, nil)
				awsError := awserr.NewRequestFailure(awserr.New("AccessDeniedException", "", errors.New("")), 403, "")
				repo.On("ListAllRolePolicyAttachments", mock.Anything).Return(nil, awsError)

				alerter.On("SendAlert", resourceaws.AwsIamRolePolicyAttachmentResourceType, alerts.NewRemoteAccessDeniedAlert(common.RemoteAWSTerraform, remoteerr.NewResourceListingErrorWithType(awsError, resourceaws.AwsIamRolePolicyAttachmentResourceType, resourceaws.AwsIamRolePolicyAttachmentResourceType), alerts.EnumerationPhase)).Return()
			},
		},
	}

	schemaRepository := testresource.InitFakeSchemaRepository("aws", "3.19.0")
	resourceaws.InitResourcesMetadata(schemaRepository)
	factory := terraform.NewTerraformResourceFactory(schemaRepository)
	deserializer := resource.NewDeserializer(factory)

	for _, c := range cases {
		t.Run(c.test, func(tt *testing.T) {
			shouldUpdate := c.dirName == *goldenfile.Update

			sess := session.Must(session.NewSessionWithOptions(session.Options{
				SharedConfigState: session.SharedConfigEnable,
			}))

			scanOptions := ScannerOptions{Deep: true}
			providerLibrary := terraform.NewProviderLibrary()
			remoteLibrary := common.NewRemoteLibrary()

			// Initialize mocks
			alerter := &mocks.AlerterInterface{}
			fakeRepo := &repository.MockIAMRepository{}
			c.mocks(fakeRepo, alerter)

			var repo repository.IAMRepository = fakeRepo
			providerVersion := "3.19.0"
			realProvider, err := terraform2.InitTestAwsProvider(providerLibrary, providerVersion)
			if err != nil {
				t.Fatal(err)
			}
			provider := terraform2.NewFakeTerraformProvider(realProvider)
			provider.WithResponse(c.dirName)

			// Replace mock by real resources if we are in update mode
			if shouldUpdate {
				err := realProvider.Init()
				if err != nil {
					t.Fatal(err)
				}
				provider.ShouldUpdate()
				repo = repository.NewIAMRepository(sess, cache.New(0))
			}

			remoteLibrary.AddEnumerator(remoteaws.NewIamRolePolicyAttachmentEnumerator(repo, factory))
			remoteLibrary.AddDetailsFetcher(resourceaws.AwsIamRolePolicyAttachmentResourceType, common.NewGenericDetailsFetcher(resourceaws.AwsIamRolePolicyAttachmentResourceType, provider, deserializer))

			testFilter := &filter.MockFilter{}
			testFilter.On("IsTypeIgnored", mock.Anything).Return(false)

			s := NewScanner(remoteLibrary, alerter, scanOptions, testFilter)
			got, err := s.Resources()
			assert.Equal(tt, c.err, err)
			if err != nil {
				return
			}
			test.TestAgainstGoldenFile(got, resourceaws.AwsIamRolePolicyAttachmentResourceType, c.dirName, provider, deserializer, shouldUpdate, tt)
			alerter.AssertExpectations(tt)
			fakeRepo.AssertExpectations(tt)
		})
	}
}

func TestIamAccessKey(t *testing.T) {

	cases := []struct {
		test    string
		dirName string
		mocks   func(*repository.MockIAMRepository, *mocks.AlerterInterface)
		wantErr error
	}{
		{
			test:    "no iam access_key",
			dirName: "aws_iam_access_key_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				users := []*iam.User{
					{
						UserName: aws.String("test-driftctl"),
					},
				}
				repo.On("ListAllUsers").Return(users, nil)
				repo.On("ListAllAccessKeys", users).Return([]*iam.AccessKeyMetadata{}, nil)
			},
			wantErr: nil,
		},
		{
			test:    "iam multiples keys for multiples users",
			dirName: "aws_iam_access_key_multiple",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				users := []*iam.User{
					{
						UserName: aws.String("test-driftctl"),
					},
				}
				repo.On("ListAllUsers").Return(users, nil)
				repo.On("ListAllAccessKeys", users).Return([]*iam.AccessKeyMetadata{
					{
						AccessKeyId: aws.String("AKIA5QYBVVD223VWU32A"),
						UserName:    aws.String("test-driftctl"),
					},
					{
						AccessKeyId: aws.String("AKIA5QYBVVD2QYI36UZP"),
						UserName:    aws.String("test-driftctl"),
					},
					{
						AccessKeyId: aws.String("AKIA5QYBVVD26EJME25D"),
						UserName:    aws.String("test-driftctl2"),
					},
					{
						AccessKeyId: aws.String("AKIA5QYBVVD2SWDFVVMG"),
						UserName:    aws.String("test-driftctl2"),
					},
				}, nil)
			},
			wantErr: nil,
		},
		{
			test:    "Cannot list iam user",
			dirName: "aws_iam_access_key_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				awsError := awserr.NewRequestFailure(awserr.New("AccessDeniedException", "", errors.New("")), 403, "")
				repo.On("ListAllUsers").Once().Return(nil, awsError)

				alerter.On("SendAlert", resourceaws.AwsIamAccessKeyResourceType, alerts.NewRemoteAccessDeniedAlert(common.RemoteAWSTerraform, remoteerr.NewResourceListingErrorWithType(awsError, resourceaws.AwsIamAccessKeyResourceType, resourceaws.AwsIamUserResourceType), alerts.EnumerationPhase)).Return()
			},
			wantErr: nil,
		},
		{
			test:    "Cannot list iam access_key",
			dirName: "aws_iam_access_key_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				repo.On("ListAllUsers").Once().Return([]*iam.User{}, nil)
				awsError := awserr.NewRequestFailure(awserr.New("AccessDeniedException", "", errors.New("")), 403, "")
				repo.On("ListAllAccessKeys", mock.Anything).Return(nil, awsError)

				alerter.On("SendAlert", resourceaws.AwsIamAccessKeyResourceType, alerts.NewRemoteAccessDeniedAlert(common.RemoteAWSTerraform, remoteerr.NewResourceListingErrorWithType(awsError, resourceaws.AwsIamAccessKeyResourceType, resourceaws.AwsIamAccessKeyResourceType), alerts.EnumerationPhase)).Return()
			},
			wantErr: nil,
		},
	}

	schemaRepository := testresource.InitFakeSchemaRepository("aws", "3.19.0")
	resourceaws.InitResourcesMetadata(schemaRepository)
	factory := terraform.NewTerraformResourceFactory(schemaRepository)
	deserializer := resource.NewDeserializer(factory)

	for _, c := range cases {
		t.Run(c.test, func(tt *testing.T) {
			shouldUpdate := c.dirName == *goldenfile.Update

			sess := session.Must(session.NewSessionWithOptions(session.Options{
				SharedConfigState: session.SharedConfigEnable,
			}))

			scanOptions := ScannerOptions{Deep: true}
			providerLibrary := terraform.NewProviderLibrary()
			remoteLibrary := common.NewRemoteLibrary()

			// Initialize mocks
			alerter := &mocks.AlerterInterface{}
			fakeRepo := &repository.MockIAMRepository{}
			c.mocks(fakeRepo, alerter)

			var repo repository.IAMRepository = fakeRepo
			providerVersion := "3.19.0"
			realProvider, err := terraform2.InitTestAwsProvider(providerLibrary, providerVersion)
			if err != nil {
				t.Fatal(err)
			}
			provider := terraform2.NewFakeTerraformProvider(realProvider)
			provider.WithResponse(c.dirName)

			// Replace mock by real resources if we are in update mode
			if shouldUpdate {
				err := realProvider.Init()
				if err != nil {
					t.Fatal(err)
				}
				provider.ShouldUpdate()
				repo = repository.NewIAMRepository(sess, cache.New(0))
			}

			remoteLibrary.AddEnumerator(remoteaws.NewIamAccessKeyEnumerator(repo, factory))
			remoteLibrary.AddDetailsFetcher(resourceaws.AwsIamAccessKeyResourceType, common.NewGenericDetailsFetcher(resourceaws.AwsIamAccessKeyResourceType, provider, deserializer))

			testFilter := &filter.MockFilter{}
			testFilter.On("IsTypeIgnored", mock.Anything).Return(false)

			s := NewScanner(remoteLibrary, alerter, scanOptions, testFilter)
			got, err := s.Resources()
			assert.Equal(tt, c.wantErr, err)
			if err != nil {
				return
			}
			test.TestAgainstGoldenFile(got, resourceaws.AwsIamAccessKeyResourceType, c.dirName, provider, deserializer, shouldUpdate, tt)
			alerter.AssertExpectations(tt)
			fakeRepo.AssertExpectations(tt)
		})
	}
}

func TestIamUserPolicyAttachment(t *testing.T) {

	cases := []struct {
		test    string
		dirName string
		mocks   func(*repository.MockIAMRepository, *mocks.AlerterInterface)
		wantErr error
	}{
		{
			test:    "no iam user policy",
			dirName: "aws_iam_user_policy_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				users := []*iam.User{
					{
						UserName: aws.String("loadbalancer"),
					},
				}
				repo.On("ListAllUsers").Return(users, nil)
				repo.On("ListAllUserPolicyAttachments", users).Return([]*repository.AttachedUserPolicy{}, nil)
			},
			wantErr: nil,
		},
		{
			test:    "iam multiples users multiple policies",
			dirName: "aws_iam_user_policy_attachment_multiple",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				users := []*iam.User{
					{
						UserName: aws.String("loadbalancer"),
					},
					{
						UserName: aws.String("loadbalancer2"),
					},
					{
						UserName: aws.String("loadbalancer3"),
					},
				}
				repo.On("ListAllUsers").Return(users, nil)
				repo.On("ListAllUserPolicyAttachments", users).Return([]*repository.AttachedUserPolicy{
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::726421854799:policy/test"),
							PolicyName: aws.String("test"),
						},
						UserName: *aws.String("loadbalancer"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::726421854799:policy/test2"),
							PolicyName: aws.String("test2"),
						},
						UserName: *aws.String("loadbalancer"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::726421854799:policy/test3"),
							PolicyName: aws.String("test3"),
						},
						UserName: *aws.String("loadbalancer"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::726421854799:policy/test4"),
							PolicyName: aws.String("test4"),
						},
						UserName: *aws.String("loadbalancer"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::726421854799:policy/test"),
							PolicyName: aws.String("test"),
						},
						UserName: *aws.String("loadbalancer2"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::726421854799:policy/test2"),
							PolicyName: aws.String("test2"),
						},
						UserName: *aws.String("loadbalancer2"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::726421854799:policy/test3"),
							PolicyName: aws.String("test3"),
						},
						UserName: *aws.String("loadbalancer2"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::726421854799:policy/test4"),
							PolicyName: aws.String("test4"),
						},
						UserName: *aws.String("loadbalancer2"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::726421854799:policy/test"),
							PolicyName: aws.String("test"),
						},
						UserName: *aws.String("loadbalancer3"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::726421854799:policy/test2"),
							PolicyName: aws.String("test2"),
						},
						UserName: *aws.String("loadbalancer3"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::726421854799:policy/test3"),
							PolicyName: aws.String("test3"),
						},
						UserName: *aws.String("loadbalancer3"),
					},
					{
						AttachedPolicy: iam.AttachedPolicy{
							PolicyArn:  aws.String("arn:aws:iam::726421854799:policy/test4"),
							PolicyName: aws.String("test4"),
						},
						UserName: *aws.String("loadbalancer3"),
					},
				}, nil)

			},
			wantErr: nil,
		},
		{
			test:    "cannot list user",
			dirName: "aws_iam_user_policy_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				awsError := awserr.NewRequestFailure(awserr.New("AccessDeniedException", "", errors.New("")), 403, "")
				repo.On("ListAllUsers").Return(nil, awsError)

				alerter.On("SendAlert", resourceaws.AwsIamUserPolicyAttachmentResourceType, alerts.NewRemoteAccessDeniedAlert(common.RemoteAWSTerraform, remoteerr.NewResourceListingErrorWithType(awsError, resourceaws.AwsIamUserPolicyAttachmentResourceType, resourceaws.AwsIamUserResourceType), alerts.EnumerationPhase)).Return()
			},
			wantErr: nil,
		},
		{
			test:    "cannot list user policies attachment",
			dirName: "aws_iam_user_policy_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				repo.On("ListAllUsers").Once().Return([]*iam.User{}, nil)
				awsError := awserr.NewRequestFailure(awserr.New("AccessDeniedException", "", errors.New("")), 403, "")
				repo.On("ListAllUserPolicyAttachments", mock.Anything).Return(nil, awsError)

				alerter.On("SendAlert", resourceaws.AwsIamUserPolicyAttachmentResourceType, alerts.NewRemoteAccessDeniedAlert(common.RemoteAWSTerraform, remoteerr.NewResourceListingErrorWithType(awsError, resourceaws.AwsIamUserPolicyAttachmentResourceType, resourceaws.AwsIamUserPolicyAttachmentResourceType), alerts.EnumerationPhase)).Return()
			},
			wantErr: nil,
		},
	}

	schemaRepository := testresource.InitFakeSchemaRepository("aws", "3.19.0")
	resourceaws.InitResourcesMetadata(schemaRepository)
	factory := terraform.NewTerraformResourceFactory(schemaRepository)
	deserializer := resource.NewDeserializer(factory)

	for _, c := range cases {
		t.Run(c.test, func(tt *testing.T) {
			shouldUpdate := c.dirName == *goldenfile.Update

			sess := session.Must(session.NewSessionWithOptions(session.Options{
				SharedConfigState: session.SharedConfigEnable,
			}))

			scanOptions := ScannerOptions{Deep: true}
			providerLibrary := terraform.NewProviderLibrary()
			remoteLibrary := common.NewRemoteLibrary()

			// Initialize mocks
			alerter := &mocks.AlerterInterface{}
			fakeRepo := &repository.MockIAMRepository{}
			c.mocks(fakeRepo, alerter)

			var repo repository.IAMRepository = fakeRepo
			providerVersion := "3.19.0"
			realProvider, err := terraform2.InitTestAwsProvider(providerLibrary, providerVersion)
			if err != nil {
				t.Fatal(err)
			}
			provider := terraform2.NewFakeTerraformProvider(realProvider)
			provider.WithResponse(c.dirName)

			// Replace mock by real resources if we are in update mode
			if shouldUpdate {
				err := realProvider.Init()
				if err != nil {
					t.Fatal(err)
				}
				provider.ShouldUpdate()
				repo = repository.NewIAMRepository(sess, cache.New(0))
			}

			remoteLibrary.AddEnumerator(remoteaws.NewIamUserPolicyAttachmentEnumerator(repo, factory))
			remoteLibrary.AddDetailsFetcher(resourceaws.AwsIamUserPolicyAttachmentResourceType, common.NewGenericDetailsFetcher(resourceaws.AwsIamUserPolicyAttachmentResourceType, provider, deserializer))

			testFilter := &filter.MockFilter{}
			testFilter.On("IsTypeIgnored", mock.Anything).Return(false)

			s := NewScanner(remoteLibrary, alerter, scanOptions, testFilter)
			got, err := s.Resources()
			assert.Equal(tt, c.wantErr, err)
			if err != nil {
				return
			}
			test.TestAgainstGoldenFile(got, resourceaws.AwsIamUserPolicyAttachmentResourceType, c.dirName, provider, deserializer, shouldUpdate, tt)
			alerter.AssertExpectations(tt)
			fakeRepo.AssertExpectations(tt)
		})
	}
}

func TestIamRolePolicy(t *testing.T) {

	cases := []struct {
		test    string
		dirName string
		mocks   func(*repository.MockIAMRepository, *mocks.AlerterInterface)
		wantErr error
	}{
		{
			test:    "no iam role policy",
			dirName: "aws_iam_role_policy_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				roles := []*iam.Role{
					{
						RoleName: aws.String("test_role"),
					},
				}
				repo.On("ListAllRoles").Return(roles, nil)
				repo.On("ListAllRolePolicies", roles).Return([]repository.RolePolicy{}, nil)
			},
			wantErr: nil,
		},
		{
			test:    "multiples roles with inline policies",
			dirName: "aws_iam_role_policy_multiple",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				roles := []*iam.Role{
					{
						RoleName: aws.String("test_role_0"),
					},
					{
						RoleName: aws.String("test_role_1"),
					},
				}
				repo.On("ListAllRoles").Return(roles, nil)
				repo.On("ListAllRolePolicies", roles).Return([]repository.RolePolicy{
					{Policy: "policy-role0-0", RoleName: "test_role_0"},
					{Policy: "policy-role0-1", RoleName: "test_role_0"},
					{Policy: "policy-role0-2", RoleName: "test_role_0"},
					{Policy: "policy-role1-0", RoleName: "test_role_1"},
					{Policy: "policy-role1-1", RoleName: "test_role_1"},
					{Policy: "policy-role1-2", RoleName: "test_role_1"},
				}, nil).Once()
			},
			wantErr: nil,
		},
		{
			test:    "Cannot list roles",
			dirName: "aws_iam_role_policy_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				awsError := awserr.NewRequestFailure(awserr.New("AccessDeniedException", "", errors.New("")), 403, "")
				repo.On("ListAllRoles").Once().Return(nil, awsError)

				alerter.On("SendAlert", resourceaws.AwsIamRolePolicyResourceType, alerts.NewRemoteAccessDeniedAlert(common.RemoteAWSTerraform, remoteerr.NewResourceListingErrorWithType(awsError, resourceaws.AwsIamRolePolicyResourceType, resourceaws.AwsIamRoleResourceType), alerts.EnumerationPhase)).Return()
			},
			wantErr: nil,
		},
		{
			test:    "cannot list role policy",
			dirName: "aws_iam_role_policy_empty",
			mocks: func(repo *repository.MockIAMRepository, alerter *mocks.AlerterInterface) {
				repo.On("ListAllRoles").Once().Return([]*iam.Role{}, nil)
				awsError := awserr.NewRequestFailure(awserr.New("AccessDeniedException", "", errors.New("")), 403, "")
				repo.On("ListAllRolePolicies", mock.Anything).Return(nil, awsError)

				alerter.On("SendAlert", resourceaws.AwsIamRolePolicyResourceType, alerts.NewRemoteAccessDeniedAlert(common.RemoteAWSTerraform, remoteerr.NewResourceListingErrorWithType(awsError, resourceaws.AwsIamRolePolicyResourceType, resourceaws.AwsIamRolePolicyResourceType), alerts.EnumerationPhase)).Return()
			},
			wantErr: nil,
		},
	}

	schemaRepository := testresource.InitFakeSchemaRepository("aws", "3.19.0")
	resourceaws.InitResourcesMetadata(schemaRepository)
	factory := terraform.NewTerraformResourceFactory(schemaRepository)
	deserializer := resource.NewDeserializer(factory)

	for _, c := range cases {
		t.Run(c.test, func(tt *testing.T) {
			shouldUpdate := c.dirName == *goldenfile.Update

			sess := session.Must(session.NewSessionWithOptions(session.Options{
				SharedConfigState: session.SharedConfigEnable,
			}))

			scanOptions := ScannerOptions{Deep: true}
			providerLibrary := terraform.NewProviderLibrary()
			remoteLibrary := common.NewRemoteLibrary()

			// Initialize mocks
			alerter := &mocks.AlerterInterface{}
			fakeRepo := &repository.MockIAMRepository{}
			c.mocks(fakeRepo, alerter)

			var repo repository.IAMRepository = fakeRepo
			providerVersion := "3.19.0"
			realProvider, err := terraform2.InitTestAwsProvider(providerLibrary, providerVersion)
			if err != nil {
				t.Fatal(err)
			}
			provider := terraform2.NewFakeTerraformProvider(realProvider)
			provider.WithResponse(c.dirName)

			// Replace mock by real resources if we are in update mode
			if shouldUpdate {
				err := realProvider.Init()
				if err != nil {
					t.Fatal(err)
				}
				provider.ShouldUpdate()
				repo = repository.NewIAMRepository(sess, cache.New(0))
			}

			remoteLibrary.AddEnumerator(remoteaws.NewIamRolePolicyEnumerator(repo, factory))
			remoteLibrary.AddDetailsFetcher(resourceaws.AwsIamRolePolicyResourceType, common.NewGenericDetailsFetcher(resourceaws.AwsIamRolePolicyResourceType, provider, deserializer))

			testFilter := &filter.MockFilter{}
			testFilter.On("IsTypeIgnored", mock.Anything).Return(false)

			s := NewScanner(remoteLibrary, alerter, scanOptions, testFilter)
			got, err := s.Resources()
			assert.Equal(tt, c.wantErr, err)
			if err != nil {
				return
			}
			test.TestAgainstGoldenFile(got, resourceaws.AwsIamRolePolicyResourceType, c.dirName, provider, deserializer, shouldUpdate, tt)
			alerter.AssertExpectations(tt)
			fakeRepo.AssertExpectations(tt)
		})
	}
}
