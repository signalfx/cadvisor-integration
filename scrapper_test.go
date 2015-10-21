package prometheustosignalfx

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
		server := httptest.NewServer(prometheus.Handler())
		scrapper := Scrapper{
			client: http.DefaultClient,
			l:      log.New(ioutil.Discard, "", 0),
		}
		ctx := context.Background()
		serverURL, err := url.Parse(server.URL)
		So(err, ShouldBeNil)
		Convey("I should be able to fetch metrics", func() {
			points, err := scrapper.Fetch(ctx, serverURL)
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
