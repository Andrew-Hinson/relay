package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var errApplyCancelled = errors.New("apply cancelled")

// confirmApply asks before Apply changes anything. --yes skips the prompt;
// without a terminal to ask on, Apply refuses rather than proceeding silently.
func confirmApply(in io.Reader, out io.Writer, yes, interactive bool) error {
	if yes {
		return nil
	}
	if !interactive {
		return errors.New("refusing to apply without confirmation: no terminal; review with relay plan and pass --yes")
	}
	fmt.Fprint(out, "\nApply these changes? Only 'yes' is accepted: ")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	if strings.TrimSpace(line) != "yes" {
		return errApplyCancelled
	}
	return nil
}

func stdinIsTerminal() bool {
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// formatReview is the Postgres side of what Apply shows before it asks:
// objects --adopt will stamp and the DDL it will run. Terraform prints its own plan.
func formatReview(adopt []planLine, sql string) string {
	var b strings.Builder
	if len(adopt) > 0 {
		b.WriteString(formatPlanDiff(planDiff{Adopt: adopt}))
	}
	if sql != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("sql\n")
		for _, line := range strings.Split(strings.TrimRight(sql, "\n"), "\n") {
			if line == "" {
				b.WriteByte('\n')
				continue
			}
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
