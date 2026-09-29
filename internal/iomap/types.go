// The IO-list adapter returns the shared inventory model; these aliases preserve its API.
package iomap

import "scheme-xml-generator/internal/domain/inventory"

type Plan = inventory.Plan
type Controller = inventory.Controller
type Rack = inventory.Rack
type Module = inventory.Module
type Channel = inventory.Channel
type Selection = inventory.Selection
