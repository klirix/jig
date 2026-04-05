package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

type cliOutput struct {
	color bool
	tty   bool
}

func newCLIOutput() cliOutput {
	stat, _ := os.Stdout.Stat()
	tty := stat != nil && (stat.Mode()&os.ModeCharDevice) != 0

	term := os.Getenv("TERM")
	if os.Getenv("NO_COLOR") != "" || term == "dumb" {
		return cliOutput{tty: tty}
	}
	if os.Getenv("FORCE_COLOR") != "" || term != "" {
		return cliOutput{color: true, tty: tty}
	}
	return cliOutput{tty: tty}
}

func (o cliOutput) style(code, text string) string {
	if !o.color || text == "" {
		return text
	}
	return "\033[" + code + "m" + text + "\033[0m"
}

func (o cliOutput) bold(text string) string   { return o.style("1", text) }
func (o cliOutput) dim(text string) string    { return o.style("2", text) }
func (o cliOutput) cyan(text string) string   { return o.style("36", text) }
func (o cliOutput) green(text string) string  { return o.style("32", text) }
func (o cliOutput) yellow(text string) string { return o.style("33", text) }

func (o cliOutput) section(title, subtitle string) {
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, o.bold(title))
	if subtitle != "" {
		fmt.Fprintln(os.Stdout, o.dim(subtitle))
	}
}

func (o cliOutput) line(label, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(os.Stdout, "%s %s\n", o.dim(label), value)
}

func (o cliOutput) success(message string) {
	fmt.Fprintf(os.Stdout, "%s %s\n", o.green("OK"), message)
}

func (o cliOutput) warning(message string) {
	fmt.Fprintf(os.Stdout, "%s %s\n", o.yellow("!"), message)
}

func (o cliOutput) table(headers []string, render func(*tabwriter.Writer)) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	headerRow := make([]string, 0, len(headers))
	for _, header := range headers {
		headerRow = append(headerRow, strings.ToUpper(header))
	}
	fmt.Fprintln(writer, strings.Join(headerRow, "\t"))
	render(writer)
	_ = writer.Flush()
}

func formatDeploymentName(prefix, name string, isRoot, isLast bool) string {
	if isRoot {
		return name
	}
	connector := "|- "
	if isLast {
		connector = "`- "
	}
	return prefix + connector + name
}

func childDeploymentPrefix(prefix string, isRoot, isLast bool) string {
	if isRoot {
		return prefix
	}
	if isLast {
		return prefix + "   "
	}
	return prefix + "|  "
}

func sortedServers(servers map[string]string) []string {
	names := make([]string, 0, len(servers))
	for server := range servers {
		names = append(names, server)
	}
	sort.Strings(names)
	return names
}

type loadingHandle struct {
	label   string
	done    chan struct{}
	wg      sync.WaitGroup
	printed bool
}

func (o cliOutput) startLoading(label string) *loadingHandle {
	handle := &loadingHandle{
		label:   label,
		done:    make(chan struct{}),
		printed: true,
	}

	if !o.tty {
		fmt.Fprintln(os.Stdout, o.dim(label+"..."))
		return handle
	}

	handle.wg.Add(1)
	go func() {
		defer handle.wg.Done()

		frames := []string{"-", "\\", "|", "/"}
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		index := 0
		for {
			fmt.Fprintf(os.Stdout, "\r%s %s", o.cyan(frames[index]), o.dim(label))
			index = (index + 1) % len(frames)

			select {
			case <-handle.done:
				fmt.Fprint(os.Stdout, "\r\033[2K")
				return
			case <-ticker.C:
			}
		}
	}()

	return handle
}

func (h *loadingHandle) stop() {
	if h == nil || !h.printed {
		return
	}
	close(h.done)
	h.wg.Wait()
}
