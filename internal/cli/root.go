package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cookiex/internal/chrome"
	cookiemodel "cookiex/internal/cookie"
	cookiediff "cookiex/internal/diff"
	exporter "cookiex/internal/export"
	"cookiex/internal/history"
	hdrs "cookiex/internal/headers"
	requestmodel "cookiex/internal/request"
	"cookiex/internal/tui"
	"cookiex/internal/vault"

	"github.com/spf13/cobra"
)

type ProfileStore interface {
	Save(profile vault.Profile) error
	Load(name string) (vault.Profile, error)
	Exists(name string) (bool, error)
	List() ([]string, error)
}

type RequestRunner interface {
	Send(context.Context, requestmodel.Spec, []cookiemodel.Cookie) (requestmodel.Response, error)
}

type Services struct {
	ConfigHome       string
	Profiles         ProfileStore
	DiscoverProfiles func(configHome string) ([]chrome.Profile, error)
	ReadCookies      func(context.Context, chrome.Profile, string) ([]cookiemodel.Cookie, error)
	Runner           RequestRunner
	Selections       SelectionStore
	Input            io.Reader
	Now              func() time.Time
}

func DefaultServices() (Services, error) {
	configHome, err := configHome()
	if err != nil {
		return Services{}, err
	}
	dataHome, err := dataHome()
	if err != nil {
		return Services{}, err
	}
	return Services{
		ConfigHome: configHome,
		Profiles: vault.New(
			filepath.Join(dataHome, "cookiex", "profiles"),
			vault.NewKeyringKeyProvider(),
		),
		DiscoverProfiles: chrome.DiscoverProfiles,
		ReadCookies: func(ctx context.Context, profile chrome.Profile, host string) ([]cookiemodel.Cookie, error) {
			decrypter := chrome.NewLinuxDecrypter(profile.Application, chrome.NewChromeSecretProvider())
			return chrome.ReadCookies(ctx, profile, host, decrypter)
		},
		Runner: requestmodel.Runner{
			Client: &http.Client{Timeout: 30 * time.Second},
		},
		Selections: NewFileSelectionStore(filepath.Join(configHome, "cookiex", "selection.json")),
		Input:      os.Stdin,
		Now:        time.Now,
	}, nil
}

func NewRootCommand(services Services) *cobra.Command {
	command := &cobra.Command{
		Use:           "cookiex",
		Short:         "Bridge authenticated Chrome cookies into terminal requests",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	command.CompletionOptions.DisableDefaultCmd = true
	command.AddCommand(
		newImportCommand(services),
		newProfilesCommand(services),
		newShowCommand(services),
		newDiffCommand(services),
		newUICommand(services),
		newPlayCommand(services),
		newExportCommand(services),
		newSendCommand(services),
		newSyncCommand(services),
	)
	return command
}

func newImportCommand(services Services) *cobra.Command {
	var profileName, chromeProfile string
	var force bool
	command := &cobra.Command{
		Use:   "import <host>",
		Short: "Import a domain-scoped cookie snapshot from Chrome",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if profileName == "" {
				return fmt.Errorf("--profile is required")
			}
			host, err := cookiemodel.NormalizeHost(args[0])
			if err != nil {
				return err
			}
			exists, err := services.Profiles.Exists(profileName)
			if err != nil {
				return err
			}
			if exists && !force {
				return fmt.Errorf("profile %q already exists; use --force to overwrite", profileName)
			}
			browserProfile, err := chooseProfile(command, services, chromeProfile)
			if err != nil {
				return err
			}
			cookies, err := services.ReadCookies(command.Context(), browserProfile, host)
			if err != nil {
				return err
			}
			now := now(services)
			createdAt := now
			if exists {
				existing, loadErr := services.Profiles.Load(profileName)
				if loadErr != nil {
					return loadErr
				}
				createdAt = existing.CreatedAt
			}
			profile := vault.Profile{
				Name:               profileName,
				Host:               host,
				Browser:            browserProfile.Browser,
				BrowserProfile:     browserProfile.Name,
				BrowserProfilePath: browserProfile.Path,
				CreatedAt:          createdAt,
				SyncedAt:           now,
				Cookies:            cookies,
			}
			if err := services.Profiles.Save(profile); err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "Imported %d cookies for %s into profile %s\n", len(cookies), host, profileName)
			return nil
		},
	}
	command.Flags().StringVarP(&profileName, "profile", "p", "", "Cookiex profile name")
	command.Flags().StringVar(&chromeProfile, "chrome-profile", "", "Chrome profile name, Browser:Name, or path")
	command.Flags().BoolVar(&force, "force", false, "overwrite an existing Cookiex profile")
	return command
}

func newProfilesCommand(services Services) *cobra.Command {
	return &cobra.Command{
		Use:   "profiles",
		Short: "List encrypted cookie profiles",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			names, err := services.Profiles.List()
			if err != nil {
				return err
			}
			sort.Strings(names)
			if len(names) == 0 {
				fmt.Fprintln(command.OutOrStdout(), "No saved profiles.")
				return nil
			}
			fmt.Fprintln(command.OutOrStdout(), "NAME\tHOST\tBROWSER PROFILE\tCOOKIES\tLAST SYNC")
			for _, name := range names {
				profile, err := services.Profiles.Load(name)
				if err != nil {
					return err
				}
				fmt.Fprintf(
					command.OutOrStdout(),
					"%s\t%s\t%s / %s\t%d\t%s\n",
					profile.Name,
					profile.Host,
					profile.Browser,
					profile.BrowserProfile,
					len(profile.Cookies),
					profile.SyncedAt.Format(time.RFC3339),
				)
			}
			return nil
		},
	}
}

func newShowCommand(services Services) *cobra.Command {
	var showValues bool
	command := &cobra.Command{
		Use:   "show <profile>",
		Short: "Show cookies in a profile (values redacted by default)",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := services.Profiles.Load(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "Profile: %s\n", profile.Name)
			fmt.Fprintf(command.OutOrStdout(), "Host: %s\n", profile.Host)
			fmt.Fprintf(command.OutOrStdout(), "Browser: %s / %s\n", profile.Browser, profile.BrowserProfile)
			fmt.Fprintf(command.OutOrStdout(), "Synced: %s\n", profile.SyncedAt.Format(time.RFC3339))
			fmt.Fprintf(command.OutOrStdout(), "Cookies: %d\n\n", len(profile.Cookies))
			fmt.Fprintln(command.OutOrStdout(), "NAME\tDOMAIN\tPATH\tFLAGS\tEXPIRES\tVALUE")
			for _, item := range profile.Cookies {
				flags := make([]string, 0, 2)
				if item.Secure {
					flags = append(flags, "secure")
				}
				if item.HTTPOnly {
					flags = append(flags, "httponly")
				}
				flagText := "-"
				if len(flags) > 0 {
					flagText = strings.Join(flags, ",")
				}
				expires := "session"
				if item.Expires != nil {
					expires = item.Expires.UTC().Format("2006-01-02")
				}
				value := "[redacted]"
				if showValues {
					value = item.Value
				}
				fmt.Fprintf(
					command.OutOrStdout(),
					"%s\t%s\t%s\t%s\t%s\t%s\n",
					item.Name,
					item.Domain,
					item.Path,
					flagText,
					expires,
					value,
				)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&showValues, "values", false, "include live cookie values")
	return command
}

func newDiffCommand(services Services) *cobra.Command {
	var showValues bool
	command := &cobra.Command{
		Use:   "diff <profile>",
		Short: "Compare a cookie snapshot to the current Chrome cookies",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := services.Profiles.Load(args[0])
			if err != nil {
				return err
			}
			browserProfile, err := resolveBrowserProfile(services, profile)
			if err != nil {
				return err
			}
			live, err := services.ReadCookies(command.Context(), browserProfile, profile.Host)
			if err != nil {
				return err
			}
			result := cookiediff.Compare(profile.Cookies, live)
			fmt.Fprint(command.OutOrStdout(), cookiediff.Format(profile.Name, profile.Host, result, showValues))
			return nil
		},
	}
	command.Flags().BoolVar(&showValues, "values", false, "include live cookie values in changes")
	return command
}

func newUICommand(services Services) *cobra.Command {
	var profileName, method string
	command := &cobra.Command{
		Use:   "ui [url]",
		Short: "Open the fullscreen Cookie Playground TUI",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			url := ""
			if len(args) == 1 {
				url = args[0]
			}
			return tui.Run(tui.Options{
				Profiles:    services.Profiles,
				Runner:      services.Runner,
				Syncer:      profileSyncer{services: services},
				History:     openPlaygroundHistory(services),
				ProfileName: profileName,
				URL:         url,
				Method:      method,
			})
		},
	}
	command.Flags().StringVarP(&profileName, "profile", "p", "", "Cookiex profile name")
	command.Flags().StringVarP(&method, "method", "X", http.MethodGet, "initial HTTP method")
	return command
}

type profileSyncer struct {
	services Services
}

func (s profileSyncer) Sync(ctx context.Context, profile vault.Profile) (vault.Profile, error) {
	return SyncProfile(ctx, s.services, profile)
}

func openPlaygroundHistory(services Services) *history.Store {
	data, err := dataHome()
	if err != nil {
		return nil
	}
	store, err := history.Open(filepath.Join(data, "cookiex", "playground.json"))
	if err != nil {
		return nil
	}
	return store
}

func newPlayCommand(services Services) *cobra.Command {
	var profileName, method, body, snippet string
	var headers map[string]string
	command := &cobra.Command{
		Use:   "play <url>",
		Short: "Send a request with cookies and show response plus client snippets",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if profileName == "" {
				return fmt.Errorf("--profile is required")
			}
			profile, err := services.Profiles.Load(profileName)
			if err != nil {
				return err
			}
			formats, err := exporter.ParseSnippetFilter(snippet)
			if err != nil {
				return err
			}
			spec := requestmodel.Spec{
				Method:  method,
				URL:     args[0],
				Headers: hdrs.Expand(hdrs.Merge(profile.Headers, headers), args[0]),
				Body:    body,
			}
			response, err := services.Runner.Send(command.Context(), spec, profile.Cookies)
			if err != nil {
				return err
			}

			out := command.OutOrStdout()
			fmt.Fprintf(out, "=== Request ===\n")
			fmt.Fprintf(out, "%s %s\n", strings.ToUpper(orDefault(method, http.MethodGet)), args[0])
			fmt.Fprintf(out, "Profile: %s (%s)\n\n", profile.Name, profile.Host)

			fmt.Fprintln(out, "=== Response ===")
			writeResponse(out, response)
			fmt.Fprintln(out)

			if len(formats) > 0 {
				fmt.Fprintln(out, "=== Client snippets ===")
				for _, format := range formats {
					code, renderErr := exporter.Render(format, spec, profile.Cookies)
					if renderErr != nil {
						return renderErr
					}
					fmt.Fprintf(out, "--- %s ---\n%s\n", exporter.FormatLabel(format), strings.TrimRight(code, "\n"))
					fmt.Fprintln(out)
				}
			}
			return nil
		},
	}
	command.Flags().StringVarP(&profileName, "profile", "p", "", "Cookiex profile name")
	command.Flags().StringVarP(&method, "method", "X", http.MethodGet, "HTTP method")
	command.Flags().StringToStringVarP(&headers, "header", "H", nil, "request header name=value")
	command.Flags().StringVarP(&body, "body", "d", "", "request body")
	command.Flags().StringVar(&snippet, "snippet", "curl", "snippet formats: curl (default), all, none, or comma list")
	return command
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func newExportCommand(services Services) *cobra.Command {
	var format, method, target, body string
	var headers map[string]string
	command := &cobra.Command{
		Use:   "export <profile>",
		Short: "Export an authenticated request as code",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := services.Profiles.Load(args[0])
			if err != nil {
				return err
			}
			if target == "" {
				target = "https://" + profile.Host + "/"
			}
			output, err := exporter.Render(
				exporter.Format(strings.ToLower(format)),
				requestmodel.Spec{Method: method, URL: target, Headers: hdrs.Expand(hdrs.Merge(profile.Headers, headers), target), Body: body},
				profile.Cookies,
			)
			if err != nil {
				return err
			}
			fmt.Fprintln(command.OutOrStdout(), output)
			return nil
		},
	}
	command.Flags().StringVarP(&format, "format", "f", "curl", "curl, python, javascript, axios, httpie, or curl_cffi")
	command.Flags().StringVarP(&method, "method", "X", http.MethodGet, "HTTP method")
	command.Flags().StringVar(&target, "url", "", "request URL (defaults to the profile host)")
	command.Flags().StringToStringVarP(&headers, "header", "H", nil, "request header name=value")
	command.Flags().StringVarP(&body, "body", "d", "", "request body")
	return command
}

func newSendCommand(services Services) *cobra.Command {
	var profileName, body string
	var headers map[string]string
	command := &cobra.Command{
		Use:   "send <method> <url>",
		Short: "Send an HTTP request using a cookie profile",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if profileName == "" {
				return fmt.Errorf("--profile is required")
			}
			profile, err := services.Profiles.Load(profileName)
			if err != nil {
				return err
			}
			response, err := services.Runner.Send(command.Context(), requestmodel.Spec{
				Method: args[0], URL: args[1], Headers: hdrs.Expand(hdrs.Merge(profile.Headers, headers), args[1]), Body: body,
			}, profile.Cookies)
			if err != nil {
				return err
			}
			writeResponse(command.OutOrStdout(), response)
			return nil
		},
	}
	command.Flags().StringVarP(&profileName, "profile", "p", "", "Cookiex profile name")
	command.Flags().StringToStringVarP(&headers, "header", "H", nil, "request header name=value")
	command.Flags().StringVarP(&body, "body", "d", "", "request body")
	return command
}

func newSyncCommand(services Services) *cobra.Command {
	return &cobra.Command{
		Use:   "sync <profile>",
		Short: "Refresh a cookie snapshot from its Chrome profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := services.Profiles.Load(args[0])
			if err != nil {
				return err
			}
			oldCount := len(profile.Cookies)
			updated, err := SyncProfile(command.Context(), services, profile)
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "Synced %s: %d → %d cookies\n", updated.Name, oldCount, len(updated.Cookies))
			return nil
		},
	}
}

// SyncProfile reloads cookies for profile from its original Chrome profile path.
func SyncProfile(ctx context.Context, services Services, profile vault.Profile) (vault.Profile, error) {
	browserProfile, err := resolveBrowserProfile(services, profile)
	if err != nil {
		return vault.Profile{}, err
	}
	cookies, err := services.ReadCookies(ctx, browserProfile, profile.Host)
	if err != nil {
		return vault.Profile{}, err
	}
	profile.Cookies = cookies
	profile.SyncedAt = now(services)
	if err := services.Profiles.Save(profile); err != nil {
		return vault.Profile{}, err
	}
	return profile, nil
}

func resolveBrowserProfile(services Services, profile vault.Profile) (chrome.Profile, error) {
	profiles, err := services.DiscoverProfiles(services.ConfigHome)
	if err != nil {
		return chrome.Profile{}, err
	}
	for _, candidate := range profiles {
		if candidate.Path == profile.BrowserProfilePath {
			return candidate, nil
		}
	}
	return chrome.Profile{}, fmt.Errorf("original Chrome profile %q is no longer available", profile.BrowserProfilePath)
}

func chooseProfile(command *cobra.Command, services Services, explicit string) (chrome.Profile, error) {
	profiles, err := services.DiscoverProfiles(services.ConfigHome)
	if err != nil {
		return chrome.Profile{}, err
	}
	input := services.Input
	if input == nil {
		input = os.Stdin
	}
	return SelectChromeProfile(profiles, explicit, services.Selections, input, command.OutOrStdout())
}

func writeResponse(output io.Writer, response requestmodel.Response) {
	fmt.Fprintf(output, "%s  %s\n", response.Status, response.Duration.Round(time.Microsecond))
	names := make([]string, 0, len(response.Headers))
	for name := range response.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range response.Headers.Values(name) {
			fmt.Fprintf(output, "%s: %s\n", name, value)
		}
	}
	fmt.Fprintln(output)

	body := response.Body
	if json.Valid(body) {
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, body, "", "  "); err == nil {
			body = formatted.Bytes()
		}
	}
	_, _ = output.Write(body)
	if len(body) > 0 && body[len(body)-1] != '\n' {
		fmt.Fprintln(output)
	}
	if response.Truncated {
		fmt.Fprintln(output, "[response body truncated]")
	}
}

func now(services Services) time.Time {
	if services.Now != nil {
		return services.Now().UTC()
	}
	return time.Now().UTC()
}

func configHome() (string, error) {
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		return value, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config"), nil
}

func dataHome() (string, error) {
	if value := os.Getenv("XDG_DATA_HOME"); value != "" {
		return value, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share"), nil
}
