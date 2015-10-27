package scrapper

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"io/ioutil"
	"log"

	"github.com/prometheus/client_golang/prometheus"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestScrapper(t *testing.T) {
	Convey("When setting up a prometheus server", t, func() {
		// Start a new http server.  Note that prometheus.Handler().  That server is
		// serving prometheus metrics
		server := httptest.NewServer(prometheus.Handler())

		// Create a prometheus scraper
		scrapper := Scrapper{
			Client: http.DefaultClient,
			L:      log.New(ioutil.Discard, "", 0),
		}
		ctx := context.Background()
		serverURL, err := url.Parse(server.URL)
		So(err, ShouldBeNil)
		Convey("I should be able to fetch metrics", func() {
			// Fetch that server's metrics
			points, err := scrapper.Fetch(ctx, serverURL, "Test cluster")
			So(err, ShouldBeNil)
			Convey("and should get a large number of points back by default", func() {
				for _, p := range points {
					t.Logf("%s\n", p.String())
				}
				So(len(points), ShouldBeGreaterThan, 40)
			})
		})
		Reset(func() {
			server.Close()
		})
	})
}
