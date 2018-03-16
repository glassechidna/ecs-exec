package common

import (
	"os"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/glassechidna/awscredcache"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
)


func AwsSession(profile, region string) *session.Session {
	provider := awscredcache.NewAwsCacheCredProvider(profile)

	creds := credentials.NewCredentials(provider.WrapInChain())

	sessOpts := session.Options{
		SharedConfigState: session.SharedConfigEnable,
		AssumeRoleTokenProvider: stscreds.StdinTokenProvider,
		Config: aws.Config{Credentials: creds},
	}

	if len(os.Getenv("ECS_EXEC_AWS_VERBOSE")) > 0 {
		sessOpts.Config.LogLevel = aws.LogLevel(aws.LogDebugWithHTTPBody)
	}

	if len(profile) > 0 {
		sessOpts.Profile = profile
	}

	sess, _ := session.NewSessionWithOptions(sessOpts)

	userAgentHandler := request.NamedHandler{
		Name: "ecs-exec.UserAgentHandler",
		Fn:   request.MakeAddToUserAgentHandler("ecs-exec", ApplicationVersion),
	}
	sess.Handlers.Build.PushBackNamed(userAgentHandler)

	if len(region) > 0 {
		sess.Config.Region = aws.String(region)
	}

	return sess
}

