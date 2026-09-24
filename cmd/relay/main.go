package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var errUsage = errors.New("usage: relay apply|plan <config.yaml>")

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errUsage
	}
	switch args[0] {
	case "apply":
		return runApply(args[1:])
	case "plan":
		return runPlan(args[1:])
	default:
		return errUsage
	}
}

func loadConfig(args []string) (configFile, clusterEnv, error) {
	file, env, err := parseApplyFlags(args)
	if err != nil {
		return configFile{}, clusterEnv{}, err
	}
	raw, err := readYAML(file)
	if err != nil {
		return configFile{}, clusterEnv{}, err
	}
	spec, err := parseConfig(raw)
	if err != nil {
		return configFile{}, clusterEnv{}, err
	}
	if err := env.validate(spec.Instance.Create); err != nil {
		return configFile{}, clusterEnv{}, err
	}
	if err := env.checkCluster(spec.Cluster); err != nil {
		return configFile{}, clusterEnv{}, err
	}
	return spec, env, nil
}

func runPlan(args []string) error {
	spec, env, err := loadConfig(args)
	if err != nil {
		return err
	}
	live, instancePresent, err := liveForPlan(spec, env)
	if err != nil {
		return err
	}
	plan, err := planApply(spec, live)
	if err != nil {
		return err
	}
	diff := diffPlan(plan, live, instancePresent)
	if env.Adopt {
		diff.Adopt = adoptClaims(spec, live)
	}
	fmt.Print(formatPlanDiff(diff))
	return nil
}

func runApply(args []string) error {
	spec, env, err := loadConfig(args)
	if err != nil {
		return err
	}
	tfDir, err := findTFDir()
	if err != nil {
		return err
	}
	root := filepath.Dir(tfDir)
	stateDir := configStateDir(root, spec.Name)
	inst := instanceName(spec)

	var login instanceLogin
	var live liveSnapshot
	if spec.Instance.Create {
		login = instanceLogin{Host: inst, User: defaultRDSUser}
	} else {
		info, err := describeDBInstance(inst, env.Region)
		if err != nil {
			return err
		}
		login, err = instanceLoginFromToken(info.Host, info.User, env.Region)
		if err != nil {
			return err
		}
		live, err = inspectOwned(login, spec, env.Adopt)
		if err != nil {
			return err
		}
	}
	plan, err := planApply(spec, live)
	if err != nil {
		return err
	}
	bindInstanceLogin(&login, &plan, login.Host, login.User)
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
		endpoint := login.Host
		if out, err := terraformOutput(tfDir, stateDir, "rds_endpoint"); err == nil && out != "" {
			endpoint = out
		}
		login, err = instanceLoginFromToken(endpoint, defaultRDSUser, env.Region)
		if err != nil {
			return err
		}
		bindInstanceLogin(&login, &plan, endpoint, defaultRDSUser)
		if _, _, err := writeApplyFiles(stateDir, renderTfvars(plan, env), renderApplySQL(plan)); err != nil {
			return err
		}
		live, err = inspectOwned(login, spec, env.Adopt)
		if err != nil {
			return err
		}
		plan, err = planApply(spec, live)
		if err != nil {
			return err
		}
		bindInstanceLogin(&login, &plan, endpoint, defaultRDSUser)
		tfvarsPath, _, err = writeApplyFiles(stateDir, renderTfvars(plan, env), renderApplySQL(plan))
		if err != nil {
			return err
		}
	}
	if env.Adopt {
		for _, l := range adoptClaims(spec, live) {
			fmt.Fprintf(os.Stderr, "adopting %s %s\n", l.Kind, l.Name)
		}
	}
	if err := applySQL(login, plan); err != nil {
		return err
	}
	if err := runTerraform(tfDir, tfEnv, "apply", "-auto-approve", "-var-file="+tfvarsPath); err != nil {
		return err
	}
	fmt.Print(formatConnection(plan.Connection))
	return nil
}

// inspectOwned reads live state and refuses objects another Config owns.
func inspectOwned(login instanceLogin, spec configFile, adopt bool) (liveSnapshot, error) {
	owner, cdc := configRoles(spec)
	live, err := inspectLive(login, databaseName(spec), []string{owner, cdc})
	if err != nil {
		return liveSnapshot{}, err
	}
	if err := checkOwnership(spec, live, adopt); err != nil {
		return liveSnapshot{}, err
	}
	return live, nil
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
