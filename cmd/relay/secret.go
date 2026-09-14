package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var errSecretNotFound = errors.New("secret not found")

const passwordLen = 32

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
	out, err := exec.Command("aws", secretARNArgs(name, region)...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if secretNotFound(s) {
			return "", fmt.Errorf("secret %s: %w", name, errSecretNotFound)
		}
		return "", fmt.Errorf("secret %s: %s", name, s)
	}
	if s == "" || s == "None" {
		return "", fmt.Errorf("secret %s: empty ARN", name)
	}
	return s, nil
}

func retrieveInstanceSecret(name, region string) (instanceCreds, error) {
	out, err := exec.Command("aws", secretValueArgs(name, region)...).Output()
	if err != nil {
		return instanceCreds{}, fmt.Errorf("secret %s: %w", name, err)
	}
	creds, err := parseInstanceSecret(out)
	if err != nil {
		return instanceCreds{}, fmt.Errorf("secret %s: %w", name, err)
	}
	return creds, nil
}

func ensureConfigSecret(name, user, region string) (created bool, creds instanceCreds, arn string, err error) {
	arn, err = describeSecretARN(name, region)
	if err == nil {
		creds, err = retrieveInstanceSecret(name, region)
		return false, creds, arn, err
	}
	if !errors.Is(err, errSecretNotFound) {
		return false, instanceCreds{}, "", err
	}
	password, err := randomPassword()
	if err != nil {
		return false, instanceCreds{}, "", err
	}
	payload, err := json.Marshal(map[string]string{"username": user, "password": password})
	if err != nil {
		return false, instanceCreds{}, "", err
	}
	out, err := exec.Command("aws", createSecretArgs(name, string(payload), region)...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if secretExistsErr(s) {
			arn, err = describeSecretARN(name, region)
			if err != nil {
				return false, instanceCreds{}, "", err
			}
			creds, err = retrieveInstanceSecret(name, region)
			return false, creds, arn, err
		}
		return false, instanceCreds{}, "", fmt.Errorf("config secret %s: %s", name, s)
	}
	if s == "" || s == "None" {
		return false, instanceCreds{}, "", fmt.Errorf("config secret %s: empty ARN", name)
	}
	return true, instanceCreds{User: user, Username: user, Password: password}, s, nil
}

func secretNotFound(s string) bool {
	return strings.Contains(s, "ResourceNotFoundException") || strings.Contains(s, "can't find the specified secret")
}

func secretExistsErr(s string) bool {
	return strings.Contains(s, "ResourceExistsException") || strings.Contains(s, "already exists")
}

func randomPassword() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	b := make([]byte, passwordLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}

func secretIDFromARN(arn string) string {
	_, name, ok := strings.Cut(arn, ":secret:")
	if !ok {
		return arn
	}
	return name
}

func bindMasterSecret(login *instanceLogin, plan *applyPlan, master instanceCreds, endpoint string) {
	login.Host = endpoint
	login.User = master.User
	login.Password = master.Password
	plan.Connection.Endpoint = endpoint
	plan.Instance.Username = master.User
}

func bindConfigSecret(plan *applyPlan, creds instanceCreds, arn string) {
	plan.Connection.User = creds.User
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

func createSecretArgs(name, secret, region string) []string {
	return awsRegionArgs(region, "secretsmanager", "create-secret", "--name", name, "--secret-string", secret, "--query", "ARN", "--output", "text")
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
