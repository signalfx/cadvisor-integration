#!/bin/bash

GO15VENDOREXPERIMENT=1
go install -v -x prometheustosignalfx/scrapper
