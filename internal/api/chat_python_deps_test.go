package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
)

func writeGARoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte("class GenericAgent: pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestParseMissingPythonModulesKeepsTopLevelNamesInOrder(t *testing.T) {
	output := `Traceback (most recent call last):
  File "<string>", line 6, in <module>
    from agentmain import GenericAgent
  File "C:\ga\llmcore.py", line 3, in <module>
    import requests
ModuleNotFoundError: No module named 'requests'
ModuleNotFoundError: No module named 'urllib3.util.retry'
ModuleNotFoundError: No module named 'requests'`
	got := parseMissingPythonModules(output)
	want := []string{"requests", "urllib3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("modules=%v want=%v", got, want)
	}
}

// Missing module names become pip arguments, so anything that is not a plain
// Python identifier must be dropped rather than forwarded to the installer.
func TestParseMissingPythonModulesRejectsNonIdentifiers(t *testing.T) {
	output := "No module named '--index-url'\nNo module named 'a b'\nNo module named ''\nNo module named 'requests'"
	if got := parseMissingPythonModules(output); strings.Join(got, ",") != "requests" {
		t.Fatalf("modules=%v want=[requests]", got)
	}
}

func TestParseMissingPythonModulesCapsPackageCount(t *testing.T) {
	var b strings.Builder
	for i := 0; i < maxRepairPackages+5; i++ {
		b.WriteString("No module named 'mod")
		b.WriteByte(byte('a' + i))
		b.WriteString("'\n")
	}
	if got := parseMissingPythonModules(b.String()); len(got) != maxRepairPackages {
		t.Fatalf("modules=%d want=%d", len(got), maxRepairPackages)
	}
}

func TestPipPackageForModuleUsesDistributionNames(t *testing.T) {
	cases := map[string]string{
		"requests":                "requests",
		"bs4":                     "beautifulsoup4",
		"Crypto":                  "pycryptodome",
		"simple_websocket_server": "simple-websocket-server",
	}
	for module, want := range cases {
		if got := pipPackageForModule(module); got != want {
			t.Fatalf("pipPackageForModule(%q)=%q want=%q", module, got, want)
		}
	}
}

func TestDiagnoseChatLLMListReportsMissingModuleAsFixable(t *testing.T) {
	root := writeGARoot(t)
	python, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	listErr := &chatLLMListError{
		Stage:  "list GA LLMs failed",
		Python: python,
		Root:   root,
		Output: "ModuleNotFoundError: No module named 'requests'",
		Err:    errors.New("exit status 1"),
	}
	diag := diagnoseChatLLMList(config.AppConfig{GARoot: root}, 0, listErr)
	if diag == nil || diag.Code != chatLLMDiagMissingModule {
		t.Fatalf("diagnosis=%#v want code=%s", diag, chatLLMDiagMissingModule)
	}
	if !diag.Fixable {
		t.Fatalf("a runnable interpreter missing requests must be fixable: %#v", diag)
	}
	if strings.Join(diag.InstallPackages, ",") != "requests" {
		t.Fatalf("packages=%v want=[requests]", diag.InstallPackages)
	}
	if !strings.Contains(diag.InstallCommand, "pip install") || !strings.Contains(diag.InstallCommand, defaultPipIndexURL) {
		t.Fatalf("install command=%q want pip install with mirror", diag.InstallCommand)
	}
	// The original message is what logs already show; keep it recoverable.
	if !strings.Contains(diag.Detail, "list GA LLMs failed") {
		t.Fatalf("detail=%q want the original error text", diag.Detail)
	}
}

// A missing module reported by an interpreter that no longer exists is not
// repairable: pip cannot run there either.
func TestDiagnoseChatLLMListWillNotOfferPipForAMissingInterpreter(t *testing.T) {
	root := writeGARoot(t)
	listErr := &chatLLMListError{
		Stage:  "list GA LLMs failed",
		Python: filepath.Join(root, "gone", "python.exe"),
		Root:   root,
		Output: "ModuleNotFoundError: No module named 'requests'",
		Err:    errors.New("exit status 1"),
	}
	diag := diagnoseChatLLMList(config.AppConfig{GARoot: root}, 0, listErr)
	if diag.Code != chatLLMDiagMissingModule || diag.Fixable {
		t.Fatalf("diagnosis=%#v want unfixable missing module", diag)
	}
}

func TestDiagnoseChatLLMListReportsUnusableGARoot(t *testing.T) {
	empty := diagnoseChatLLMList(config.AppConfig{}, 0, errors.New("boom"))
	if empty.Code != chatLLMDiagGARoot || empty.Hint == "" {
		t.Fatalf("diagnosis=%#v want ga root code with a hint", empty)
	}
	bare := diagnoseChatLLMList(config.AppConfig{GARoot: t.TempDir()}, 0, errors.New("boom"))
	if bare.Code != chatLLMDiagGARoot {
		t.Fatalf("diagnosis=%#v want ga root code for a directory without agentmain.py", bare)
	}
	if bare.Fixable {
		t.Fatalf("a bad GA root is not fixable by pip: %#v", bare)
	}
}

func TestDiagnoseChatLLMListReportsUnusablePython(t *testing.T) {
	root := writeGARoot(t)
	listErr := &chatLLMListError{
		Stage:  "list GA LLMs failed",
		Python: filepath.Join(root, "missing-python"),
		Root:   root,
		Output: "The system cannot find the file specified.",
		Err:    errors.New("exec: file does not exist"),
	}
	diag := diagnoseChatLLMList(config.AppConfig{GARoot: root}, 0, listErr)
	if diag.Code != chatLLMDiagPython {
		t.Fatalf("diagnosis=%#v want code=%s", diag, chatLLMDiagPython)
	}
}

// A GA that started cleanly and returned nothing is a configuration gap, not a
// broken environment; the chat UI must send those users to the models page.
func TestDiagnoseChatLLMListReportsMissingModelConfig(t *testing.T) {
	root := writeGARoot(t)
	diag := diagnoseChatLLMList(config.AppConfig{GARoot: root, PythonPath: "python"}, 0, nil)
	if diag == nil || diag.Code != chatLLMDiagNoModels {
		t.Fatalf("diagnosis=%#v want code=%s", diag, chatLLMDiagNoModels)
	}
	if diag.Fixable || diag.Detail != "" {
		t.Fatalf("a configuration gap has no python failure to report: %#v", diag)
	}
}

func TestDiagnoseChatLLMListStaysSilentWhenModelsExist(t *testing.T) {
	if diag := diagnoseChatLLMList(config.AppConfig{GARoot: writeGARoot(t)}, 3, nil); diag != nil {
		t.Fatalf("diagnosis=%#v want nil when models were listed", diag)
	}
}

// The typed error replaced a flat fmt.Errorf; the text logs and the chat state
// warning show must not change.
func TestChatLLMListErrorKeepsItsLegacyMessage(t *testing.T) {
	err := &chatLLMListError{Stage: "list GA LLMs failed", Output: "  boom  \n", Err: errors.New("exit status 1")}
	if got := err.Error(); got != "list GA LLMs failed: exit status 1: boom" {
		t.Fatalf("error=%q", got)
	}
	if !errors.Is(err, err.Err) {
		t.Fatal("wrapped cause should stay reachable through errors.Is")
	}
}

// The chat UI can only explain an empty picker if /api/chat/state ships the
// diagnosis alongside the empty list.
func TestChatStateShipsADiagnosisWithAnEmptyModelList(t *testing.T) {
	s := newChatLoopTestServer(t)
	rec := httptest.NewRecorder()
	s.chatState(rec, httptest.NewRequest(http.MethodGet, "/api/chat/state", nil), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		LLMs    []map[string]interface{} `json:"llms"`
		Backend struct {
			Warning   string `json:"warning"`
			Diagnosis struct {
				Code    string `json:"code"`
				Hint    string `json:"hint"`
				Fixable bool   `json:"fixable"`
			} `json:"diagnosis"`
		} `json:"backend"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.LLMs) != 0 {
		t.Fatalf("llms=%v want empty for a GA root without agentmain.py", payload.LLMs)
	}
	if payload.Backend.Warning == "" {
		t.Fatal("backend.warning should still carry the raw failure")
	}
	if payload.Backend.Diagnosis.Code != chatLLMDiagGARoot || payload.Backend.Diagnosis.Hint == "" {
		t.Fatalf("diagnosis=%#v want a ga root code with a hint", payload.Backend.Diagnosis)
	}
	if payload.Backend.Diagnosis.Fixable {
		t.Fatal("a GA root problem must not advertise a one-click pip repair")
	}
}

func TestChatDiagnosisPayloadOmitsEmptyFields(t *testing.T) {
	if chatDiagnosisPayload(nil) != nil {
		t.Fatal("nil diagnosis should produce no payload")
	}
	payload := chatDiagnosisPayload(&chatLLMDiagnosis{Code: chatLLMDiagNoModels, Hint: "configure a model"})
	for _, key := range []string{"missing_modules", "install_packages", "install_command", "detail"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("payload should omit empty %q: %#v", key, payload)
		}
	}
	if payload["fixable"] != false || payload["code"] != chatLLMDiagNoModels {
		t.Fatalf("payload=%#v", payload)
	}
}
