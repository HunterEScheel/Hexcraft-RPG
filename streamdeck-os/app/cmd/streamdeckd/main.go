// streamdeckd is the only application of StreamDeck OS: it maps Stream Deck
// key presses to Govee lighting scenes and Spotify playback.
//
//	streamdeckd [run]          run the appliance daemon (default)
//	streamdeckd console        tty1: installer on first boot, then status screen
//	streamdeckd setup          run the installer (Govee key, Spotify login, Wi-Fi)
//	streamdeckd check          validate config and credentials
//	streamdeckd govee-list     list Govee devices and their scene names
//	streamdeckd spotify-devices list Spotify Connect devices
//	streamdeckd spotify-auth   one-time login to get a Spotify refresh token
//	streamdeckd press          run one button's actions without a deck
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"streamdeckd/internal/config"
)

var version = "dev"

// The layout lives on the FAT boot partition so it can be edited from any
// computer; images without one fall back to /etc.
func defaultConfigPath() string {
	for _, p := range []string{"/boot/streamdeck/config.json", "/etc/streamdeck/config.json"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "/boot/streamdeck/config.json"
}

func main() {
	log.SetFlags(log.LstdFlags)
	cmd, args := "run", os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}

	defaultConfig := defaultConfigPath()
	secretsFlag := func(fs *flag.FlagSet) *string {
		return fs.String("secrets", config.DefaultSecretsPath, "secrets file written by the installer")
	}
	var err error
	switch cmd {
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		cfg := fs.String("config", defaultConfig, "button layout file")
		sec := secretsFlag(fs)
		fs.Parse(args)
		err = runDaemon(*cfg, *sec)
	case "console", "setup":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		cfg := fs.String("config", defaultConfig, "button layout file")
		sec := secretsFlag(fs)
		fs.Parse(args)
		err = runConsole(*cfg, *sec, cmd == "setup")
	case "check":
		fs := flag.NewFlagSet("check", flag.ExitOnError)
		cfg := fs.String("config", defaultConfig, "config file")
		sec := secretsFlag(fs)
		fs.Parse(args)
		err = runCheck(context.Background(), *cfg, *sec)
	case "govee-list":
		fs := flag.NewFlagSet("govee-list", flag.ExitOnError)
		sec := secretsFlag(fs)
		key := fs.String("api-key", "", "Govee API key (overrides secrets)")
		fs.Parse(args)
		err = runGoveeList(context.Background(), *sec, *key)
	case "spotify-devices":
		fs := flag.NewFlagSet("spotify-devices", flag.ExitOnError)
		sec := secretsFlag(fs)
		fs.Parse(args)
		err = runSpotifyDevices(context.Background(), *sec)
	case "spotify-auth":
		fs := flag.NewFlagSet("spotify-auth", flag.ExitOnError)
		id := fs.String("client-id", "", "Spotify app client ID")
		secret := fs.String("client-secret", "", "Spotify app client secret")
		fs.Parse(args)
		err = runSpotifyAuth(*id, *secret)
	case "press":
		fs := flag.NewFlagSet("press", flag.ExitOnError)
		cfg := fs.String("config", defaultConfig, "config file")
		sec := secretsFlag(fs)
		page := fs.String("page", "", "page name (default: start_page)")
		key := fs.Int("key", -1, "key number")
		fs.Parse(args)
		err = runPress(context.Background(), *cfg, *sec, *page, *key)
	case "version":
		fmt.Println(version)
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stderr, "usage: streamdeckd [run|console|setup|check|govee-list|spotify-devices|spotify-auth|press|version] [flags]")
	default:
		err = fmt.Errorf("unknown command %q (try: streamdeckd help)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
