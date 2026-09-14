package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] != "apply" {
		return errors.New("usage: relay apply -f <config.yaml>")
	}
	file, env, err := parseApplyFlags(args[1:])
	if err != nil {
		return err
	}
	raw, err := readYAML(file)
	if err != nil {
		return err
	}
	spec, err := parseConfig(raw)
	if err != nil {
		return err
	}
	if err := env.validate(spec.Instance.Create); err != nil {
		return err
	}
	tfDir, err := findTFDir()
	if err != nil {
		return err
	}
	creds, err := retrieveInstanceSecret(instanceName(spec), env.Region)
	if err != nil {
		return err
	}
	root := filepath.Dir(tfDir)
	stateDir := configStateDir(root, spec.Name)
	endpoint, err := resolveEndpoint(creds, instanceName(spec), env.Region, spec.Instance.Create)
	if err != nil {
		return err
	}
	login := instanceLogin{Host: endpoint, User: creds.User, Password: creds.Password}

	var live liveSnapshot
	if spec.Instance.Create {
		live = liveSnapshot{}
	} else {
		live, err = inspectLive(login, databaseName(spec))
		if err != nil {
			return err
		}
	}
	plan, err := planApply(spec, live)
	if err != nil {
		return err
	}
	plan.Connection.Endpoint = endpoint
	plan.Instance.Username = creds.User
	tfvarsPath, _, err := writeApplyFiles(stateDir, renderTfvars(plan, env), renderApplySQL(plan))
	if err != nil {
		return err
	}
	if err := initApplyBackend(tfDir, stateDir, env, spec.Cluster, spec.Name); err != nil {
		return err
	}
	tfEnv := terraformEnv(stateDir)
	if spec.Instance.Create {
		if err := runTerraform(tfDir, tfEnv, "apply", "-auto-approve", "-var-file="+tfvarsPath, "-target=module.rds"); err != nil {
			return err
		}
		if out, err := terraformOutput(tfDir, stateDir, "rds_endpoint"); err == nil && out != "" {
			endpoint = out
		}
		arn, err := terraformOutput(tfDir, stateDir, "rds_master_secret_arn")
		if err != nil {
			return fmt.Errorf("rds master secret: %w", err)
		}
		if arn == "" {
			return errors.New("rds master secret: empty")
		}
		master, err := retrieveInstanceSecret(arn, env.Region)
		if err != nil {
			return err
		}
		bindMasterSecret(&login, &plan, master, endpoint)
		if _, _, err := writeApplyFiles(stateDir, renderTfvars(plan, env), renderApplySQL(plan)); err != nil {
			return err
		}
		live, err = inspectLive(login, databaseName(spec))
		if err != nil {
			return err
		}
		plan, err = planApply(spec, live)
		if err != nil {
			return err
		}
		bindMasterSecret(&login, &plan, master, endpoint)
		tfvarsPath, _, err = writeApplyFiles(stateDir, renderTfvars(plan, env), renderApplySQL(plan))
		if err != nil {
			return err
		}
	}
	created, roleCreds, arn, err := ensureConfigSecret(plan.Connection.Secret, plan.Connection.User, env.Region)
	if err != nil {
		return err
	}
	if roleCreds.User != plan.Connection.User {
		return fmt.Errorf("config secret %s user %q does not match role %q", plan.Connection.Secret, roleCreds.User, plan.Connection.User)
	}
	bindConfigSecret(&plan, roleCreds, arn)
	tfvarsPath, _, err = writeApplyFiles(stateDir, renderTfvars(plan, env), renderApplySQL(plan))
	if err != nil {
		return err
	}
	roleLogin := instanceLogin{Host: login.Host, User: roleCreds.User, Password: roleCreds.Password}
	if err := applySQL(login, roleLogin, plan, created); err != nil {
		return err
	}
	if err := runTerraform(tfDir, tfEnv, "apply", "-auto-approve", "-var-file="+tfvarsPath); err != nil {
		return err
	}
	fmt.Print(formatConnection(plan.Connection))
	return nil
}

func readYAML(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err == nil {
		return raw, nil
	}
	tfDir, ferr := findTFDir()
	if ferr != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(filepath.Dir(tfDir), path))
}
