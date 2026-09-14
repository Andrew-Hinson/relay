package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type instanceCreds struct {
	User     string `json:"user"`
	Username string `json:"username"`
	Password string `json:"password"`
	Host     string `json:"host"`
}

func parseInstanceSecret(raw []byte) (instanceCreds, error) {
	var creds instanceCreds
	if err := json.Unmarshal(raw, &creds); err != nil {
		return instanceCreds{}, err
	}
	if creds.User == "" {
		creds.User = creds.Username
	}
	if creds.User == "" || creds.Password == "" {
		return instanceCreds{}, fmt.Errorf("missing user or password")
	}
	return creds, nil
}

func describeSecretARN(name, region string) (string, error) {
	out, err := exec.Command("aws", secretARNArgs(name, region)...).Output()
	if err != nil {
		return "", fmt.Errorf("instance secret %s: %w", name, err)
	}
	arn := strings.TrimSpace(string(out))
	if arn == "" || arn == "None" {
		return "", fmt.Errorf("instance secret %s: empty ARN", name)
	}
	return arn, nil
}

func retrieveInstanceSecret(name, region string) (instanceCreds, error) {
	out, err := exec.Command("aws", secretValueArgs(name, region)...).Output()
	if err != nil {
		return instanceCreds{}, fmt.Errorf("instance secret %s: %w", name, err)
	}
	creds, err := parseInstanceSecret(out)
	if err != nil {
		return instanceCreds{}, fmt.Errorf("instance secret %s: %w", name, err)
	}
	return creds, nil
}

func secretIDFromARN(arn string) string {
	_, name, ok := strings.Cut(arn, ":secret:")
	if !ok {
		return arn
	}
	return name
}

func bindMasterSecret(login *instanceLogin, plan *applyPlan, master instanceCreds, endpoint, arn string) {
	login.Host = endpoint
	login.User = master.User
	login.Password = master.Password
	plan.Connection.Endpoint = endpoint
	plan.Connection.User = master.User
	plan.Connection.Secret = secretIDFromARN(arn)
	plan.Connection.SecretARN = arn
	plan.Connection.SecretUserKey = "username"
}

func resolveEndpoint(creds instanceCreds, instance, region string, create bool) (string, error) {
	if creds.Host != "" {
		return creds.Host, nil
	}
	if create {
		return instance, nil
	}
	out, err := exec.Command("aws", describeDBInstanceArgs(instance, region)...).Output()
	if err != nil {
		return "", fmt.Errorf("instance endpoint %s: %w", instance, err)
	}
	host := strings.TrimSpace(string(out))
	if host == "" || host == "None" {
		return "", fmt.Errorf("instance endpoint %s is empty", instance)
	}
	return host, nil
}

func secretARNArgs(name, region string) []string {
	return awsRegionArgs(region, "secretsmanager", "describe-secret", "--secret-id", name, "--query", "ARN", "--output", "text")
}

func secretValueArgs(name, region string) []string {
	return awsRegionArgs(region, "secretsmanager", "get-secret-value", "--secret-id", name, "--query", "SecretString", "--output", "text")
}

func describeDBInstanceArgs(instance, region string) []string {
	return awsRegionArgs(region, "rds", "describe-db-instances", "--db-instance-identifier", instance, "--query", "DBInstances[0].Endpoint.Address", "--output", "text")
}

func awsRegionArgs(region string, args ...string) []string {
	if region == "" {
		return args
	}
	out := make([]string, 0, len(args)+2)
	out = append(out, args...)
	return append(out, "--region", region)
}
