package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type dbInstanceInfo struct {
	Host string `json:"host"`
	User string `json:"user"`
	IAM  bool   `json:"iam"`
}

func parseDBInstance(raw []byte) (dbInstanceInfo, error) {
	var info dbInstanceInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return dbInstanceInfo{}, err
	}
	if info.Host == "" || info.User == "" {
		return dbInstanceInfo{}, fmt.Errorf("instance endpoint or master user is empty")
	}
	if !info.IAM {
		return dbInstanceInfo{}, errors.New("IAM database authentication is off")
	}
	return info, nil
}

func describeDBInstance(name, region string) (dbInstanceInfo, error) {
	out, err := exec.Command("aws", describeDBInstanceArgs(name, region)...).Output()
	if err != nil {
		return dbInstanceInfo{}, fmt.Errorf("instance %s: %s", name, awsErrMsg(out, err))
	}
	info, err := parseDBInstance(out)
	if err != nil {
		return dbInstanceInfo{}, fmt.Errorf("instance %s: %w", name, err)
	}
	return info, nil
}

func generateDBAuthToken(host, user, region string) (string, error) {
	out, err := exec.Command("aws", generateDBAuthTokenArgs(host, user, region)...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return "", fmt.Errorf("auth token %s: %s", host, s)
	}
	if s == "" {
		return "", fmt.Errorf("auth token %s: empty", host)
	}
	return s, nil
}

func instanceLoginFromToken(host, user, region string) (instanceLogin, error) {
	token, err := generateDBAuthToken(host, user, region)
	if err != nil {
		return instanceLogin{}, err
	}
	return instanceLogin{Host: host, User: user, Password: token}, nil
}

func bindInstanceLogin(login *instanceLogin, plan *applyPlan, host, user string) {
	login.Host = host
	login.User = user
	plan.Connection.Endpoint = host
	plan.Instance.Username = user
}

func describeDBInstanceArgs(instance, region string) []string {
	return awsRegionArgs(region, "rds", "describe-db-instances", "--db-instance-identifier", instance, "--query", "DBInstances[0].{host:Endpoint.Address,user:MasterUsername,iam:IAMDatabaseAuthenticationEnabled}", "--output", "json")
}

func generateDBAuthTokenArgs(host, user, region string) []string {
	return awsRegionArgs(region, "rds", "generate-db-auth-token", "--hostname", host, "--port", postgresPort, "--username", user)
}

func awsRegionArgs(region string, args ...string) []string {
	if region == "" {
		return args
	}
	out := make([]string, 0, len(args)+2)
	out = append(out, args...)
	return append(out, "--region", region)
}

func awsErrMsg(out []byte, err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return strings.TrimSpace(string(ee.Stderr))
	}
	s := strings.TrimSpace(string(out))
	if s != "" {
		return s
	}
	return err.Error()
}
