package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hui1601/Quicky/internal/webapp"
)

const (
	defaultAddress       = "127.0.0.1:8090"
	defaultDaemonAddress = "127.0.0.1:47892"
	controlName          = "control.json"
)

type controlInfo struct {
	PID       int       `json:"pid"`
	Address   string    `json:"address"`
	Token     string    `json:"token"`
	LogPath   string    `json:"logPath,omitempty"`
	StartedAt time.Time `json:"startedAt"`
}

func run(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return serveCommand(args)
	}

	switch args[0] {
	case "serve":
		return serveCommand(args[1:])
	case "daemon":
		return daemonCommand(args[1:])
	case "start":
		return startCommand(args[1:])
	case "stop":
		return stopCommand(args[1:])
	case "restart":
		return restartCommand(args[1:])
	case "status":
		return statusCommand(args[1:])
	case "logs":
		return logsCommand(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("comando desconhecido %q; use 'quicky help'", args[0])
	}
}

func serveCommand(args []string) error {
	return serveWithDefaults("serve", defaultAddress, true, args)
}

func daemonCommand(args []string) error {
	return serveWithDefaults("daemon", defaultDaemonAddress, false, args)
}

func serveWithDefaults(name, fallbackAddr string, fallbackOpen bool, args []string) error {
	addr, openBrowser, err := parseServeFlags(name, fallbackAddr, fallbackOpen, args)
	if err != nil {
		return err
	}

	token := os.Getenv("QUICKY_CONTROL_TOKEN")
	if token == "" {
		token, err = newToken()
		if err != nil {
			return err
		}
	}
	return serve(addr, openBrowser, token, "")
}

func parseServeFlags(name, fallbackAddr string, fallbackOpen bool, args []string) (string, bool, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	address := flags.String("addr", fallbackAddr, "endereço HTTP local")
	openBrowser := flags.Bool("open", fallbackOpen, "abrir a interface no navegador")
	if err := flags.Parse(args); err != nil {
		return "", false, err
	}
	return *address, *openBrowser, nil
}

func startCommand(args []string) error {
	flags := flag.NewFlagSet("start", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	address := flags.String("addr", defaultAddress, "endereço HTTP local")
	openBrowser := flags.Bool("open", true, "abrir a interface no navegador")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if info, err := readControl(); err == nil && serverHealthy(info) {
		fmt.Printf("QuiCkY já está rodando em %s (PID %d)\n", serverURL(info.Address), info.PID)
		if *openBrowser {
			openURL(serverURL(info.Address))
		}
		return nil
	}
	_ = os.Remove(controlPath())

	token, err := newToken()
	if err != nil {
		return err
	}
	logPath, err := serverLogPath()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("abrir log: %w", err)
	}
	defer logFile.Close()

	executable, err := os.Executable()
	if err != nil {
		return err
	}
	command := exec.Command(executable, "serve", "-addr", *address, "-open=false")
	command.Env = append(os.Environ(), "QUICKY_CONTROL_TOKEN="+token)
	command.Stdout = logFile
	command.Stderr = logFile
	command.Stdin = nil
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return fmt.Errorf("iniciar servidor: %w", err)
	}
	pid := command.Process.Pid
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("liberar processo: %w", err)
	}

	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		info, readErr := readControl()
		if readErr == nil && info.PID == pid && serverHealthy(info) {
			fmt.Printf("QuiCkY iniciado em %s (PID %d)\n", serverURL(info.Address), pid)
			fmt.Printf("Log: %s\n", info.LogPath)
			if *openBrowser {
				openURL(serverURL(info.Address))
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("o servidor não iniciou; consulte %s", logPath)
}

func stopCommand(args []string) error {
	flags := flag.NewFlagSet("stop", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	timeout := flags.Duration("timeout", 5*time.Second, "tempo máximo de encerramento")
	if err := flags.Parse(args); err != nil {
		return err
	}
	return stop(*timeout, true)
}

func stop(timeout time.Duration, printResult bool) error {
	info, err := readControl()
	if errors.Is(err, os.ErrNotExist) {
		if printResult {
			fmt.Println("QuiCkY já está parado")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !serverHealthy(info) {
		_ = os.Remove(controlPath())
		if printResult {
			fmt.Println("QuiCkY já está parado; registro antigo removido")
		}
		return nil
	}

	request, err := http.NewRequest(http.MethodPost, serverURL(info.Address)+"/api/shutdown", nil)
	if err != nil {
		return err
	}
	request.Header.Set("X-Quicky-Token", info.Token)
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("solicitar encerramento: %w", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("servidor recusou o encerramento: HTTP %d", response.StatusCode)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !serverHealthy(info) {
			if printResult {
				fmt.Println("QuiCkY encerrado")
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("tempo esgotado aguardando o servidor encerrar")
}

func restartCommand(args []string) error {
	if err := stop(5*time.Second, false); err != nil {
		return err
	}
	return startCommand(args)
}

func statusCommand(args []string) error {
	if len(args) != 0 {
		return errors.New("status não recebe argumentos")
	}
	info, err := readControl()
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("Status: parado")
		return nil
	}
	if err != nil {
		return err
	}
	if !serverHealthy(info) {
		fmt.Printf("Status: parado (registro antigo do PID %d)\n", info.PID)
		return nil
	}
	fmt.Println("Status: rodando")
	fmt.Printf("URL: %s\n", serverURL(info.Address))
	fmt.Printf("PID: %d\n", info.PID)
	fmt.Printf("Desde: %s\n", info.StartedAt.Local().Format("02/01/2006 15:04:05"))
	fmt.Printf("Log: %s\n", info.LogPath)
	return nil
}

func logsCommand(args []string) error {
	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	follow := flags.Bool("f", false, "acompanhar novas mensagens")
	if err := flags.Parse(args); err != nil {
		return err
	}
	path, err := serverLogPath()
	if err != nil {
		return err
	}
	tailArgs := []string{"-n", "80"}
	if *follow {
		tailArgs = append(tailArgs, "-f")
	}
	tailArgs = append(tailArgs, path)
	command := exec.Command("tail", tailArgs...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func serve(address string, openBrowser bool, token, logPath string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("escutar em %s: %w", address, err)
	}
	defer listener.Close()

	panel, err := webapp.New(token)
	if err != nil {
		return err
	}
	panel.StartAutoConnect()
	server := &http.Server{
		Handler:           panel.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	actualAddress := listener.Addr().String()
	if logPath == "" {
		logPath, _ = serverLogPath()
	}
	info := controlInfo{
		PID:       os.Getpid(),
		Address:   actualAddress,
		Token:     token,
		LogPath:   logPath,
		StartedAt: time.Now(),
	}
	if err := writeControl(info); err != nil {
		return err
	}
	defer removeOwnControl(info)

	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()
	url := serverURL(actualAddress)
	log.Printf("QuiCkY disponível em %s", url)
	if openBrowser {
		openURL(url)
	}

	stopSignals := make(chan os.Signal, 1)
	signal.Notify(stopSignals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopSignals)
	select {
	case received := <-stopSignals:
		log.Printf("encerrando após %s", received)
	case <-panel.ShutdownRequested():
		log.Print("encerramento solicitado pela CLI")
	case serveErr := <-serverErrors:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	shutdownErr := server.Shutdown(ctx)
	closeErr := panel.Close()
	return errors.Join(shutdownErr, closeErr)
}

func controlPath() string {
	directory := os.Getenv("XDG_RUNTIME_DIR")
	if directory == "" {
		directory = filepath.Join(os.TempDir(), "quicky-"+strconv.Itoa(os.Getuid()))
	} else {
		directory = filepath.Join(directory, "quicky")
	}
	return filepath.Join(directory, controlName)
}

func serverLogPath() (string, error) {
	directory := os.Getenv("XDG_STATE_HOME")
	if directory == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		directory = filepath.Join(home, ".local", "state")
	}
	directory = filepath.Join(directory, "quicky")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(directory, "server.log"), nil
}

func writeControl(info controlInfo) error {
	path := controlPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("criar diretório de runtime: %w", err)
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func readControl() (controlInfo, error) {
	data, err := os.ReadFile(controlPath())
	if err != nil {
		return controlInfo{}, err
	}
	var info controlInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return controlInfo{}, fmt.Errorf("registro de runtime inválido: %w", err)
	}
	return info, nil
}

func removeOwnControl(expected controlInfo) {
	current, err := readControl()
	if err == nil && current.PID == expected.PID && current.Token == expected.Token {
		_ = os.Remove(controlPath())
	}
}

func serverHealthy(info controlInfo) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	response, err := client.Get(serverURL(info.Address) + "/api/status")
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func serverURL(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "http://" + address
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func newToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func openURL(url string) {
	if err := exec.Command("xdg-open", url).Start(); err != nil {
		log.Printf("abra %s no navegador", url)
	}
}

func printUsage() {
	fmt.Print(`QuiCkY Control - painel QCY para Linux

Uso:
  quicky-control start [opções]    inicia em segundo plano
  quicky-control stop              encerra com segurança
  quicky-control restart [opções]  reinicia o servidor
  quicky-control status            mostra URL, PID e log
  quicky-control logs [-f]         mostra os logs
  quicky-control serve [opções]    executa em primeiro plano
  quicky-control daemon [opções]   executa como daemon (padrão 127.0.0.1:47892, sem navegador)

Opções de start, restart, serve e daemon:
  -addr 127.0.0.1:8090  endereço local da interface (daemon: 127.0.0.1:47892)
  -open=true            abrir o navegador

Sem comando, o comportamento é igual a "serve".
`)
}
