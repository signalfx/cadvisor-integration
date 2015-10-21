package prometheustosignalfx
import (
	"os"
	"fmt"
)

type prometheusScraper struct {
}

var instance = prometheusScraper{}

func main() {
	if err := instance.main(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
}


func (p *prometheusScraper) main() error {
	// TODO: Load from a JSON file endpoints to scrap and how frequently to scrap them
	// TODO: Scrape the endpoints in a loop
	// TODO: Send datapoint.DataPoint to signalfx as protocol buffers
	return nil
}
