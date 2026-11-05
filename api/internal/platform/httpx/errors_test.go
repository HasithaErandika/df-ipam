package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func parse(query string) Page {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	return ParsePage(c)
}

func TestParsePage(t *testing.T) {
	for query, want := range map[string]Page{
		"":                       {1, 50},
		"page=3&pageSize=25":     {3, 25},
		"page=0&pageSize=0":      {1, 50},
		"page=-2&pageSize=-1":    {1, 50},
		"page=abc&pageSize=xyz":  {1, 50},
		"pageSize=5000":          {1, 200},
		"page=99999999999999999": {1_000_000, 50},
		"page=5000000":           {1_000_000, 50},
	} {
		if got := parse(query); got != want {
			t.Errorf("%q: got %+v want %+v", query, got, want)
		}
	}
	if got := (Page{3, 25}).Offset(); got != 50 {
		t.Fatal(got)
	}
}

func TestMeta(t *testing.T) {
	for _, c := range []struct {
		page  Page
		total int64
		want  PageMeta
	}{
		{Page{1, 50}, 0, PageMeta{1, 50, 0, 0, false, false}},
		{Page{1, 50}, 50, PageMeta{1, 50, 50, 1, false, false}},
		{Page{1, 50}, 51, PageMeta{1, 50, 51, 2, true, false}},
		{Page{2, 50}, 51, PageMeta{2, 50, 51, 2, false, true}},
		{Page{9, 50}, 51, PageMeta{9, 50, 51, 2, false, true}},
		{Page{1, 0}, 0, PageMeta{1, 0, 0, 0, false, false}},
	} {
		if got := c.page.Meta(c.total); got != c.want {
			t.Errorf("%+v total %d: got %+v", c.page, c.total, got)
		}
	}
}
