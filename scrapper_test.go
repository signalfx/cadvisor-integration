package prometheus_scrapper

import (
	"github.com/prometheus/client_golang/prometheus"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestScrapper(t *testing.T) {
	Convey("When setting up a prometheus server", t, func() {
		server := httptest.NewServer(prometheus.Handler())
		scrapper := Scrapper{
			client: http.DefaultClient,
		}
		ctx := context.Background()
		serverURL, err := url.Parse(server.URL)
		So(err, ShouldBeNil)
		Convey("I should be able to fetch metrics", func() {
			points, err := scrapper.Fetch(ctx, serverURL)
			So(err, ShouldBeNil)
			Convey("and should get 46 by default back", func() {
				So(len(points), ShouldEqual, 46)
				for _, p := range points {
					t.Logf("%s\n", p.String())
				}
			})
		})
		Reset(func() {
			server.Close()
		})
	})
}
