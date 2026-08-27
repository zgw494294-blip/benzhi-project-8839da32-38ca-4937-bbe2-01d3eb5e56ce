package httpui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"stage-rigging-safety-release/internal/application"
	"stage-rigging-safety-release/internal/storage"
	"testing"
)

func TestWorkbenchAndCampaignAPI(t *testing.T) {
	repo, err := storage.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	server := httptest.NewServer(New(application.New(repo)).Handler())
	defer server.Close()
	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("工作台响应异常: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	resp.Body.Close()
	body, _ := json.Marshal(application.CreateCampaignCommand{ID: "WEB-1", TheatreName: "页面测试剧场", InspectionYear: 2026, LeadInspector: "测试员", IdempotencyKey: "web-create", Actor: "测试员", Role: "inspector"})
	resp, err = http.Post(server.URL+"/api/v1/campaigns", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("创建 API 返回 %d", resp.StatusCode)
	}
	resp2, err := http.Get(server.URL + "/api/v1/campaigns/WEB-1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("任务详情返回 %d", resp2.StatusCode)
	}
}
