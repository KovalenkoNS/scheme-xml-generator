// Независимая от формата входа модель назначений ПЛК: группы, физические модули и каналы.
package assignments

type Plan struct {
	Groups   []Group  `json:"groups"`
	Warnings []string `json:"warnings"`
}

type Group struct {
	Key            string   `json:"key"`
	ControllerName string   `json:"controllerName"`
	Kind           string   `json:"kind"`
	Prefix         string   `json:"prefix"`
	POUName        string   `json:"pouName"`
	Modules        []Module `json:"modules"`
}

type Module struct {
	Name               string    `json:"name"`
	Type               string    `json:"type"`
	ObjectType         string    `json:"objectType"`
	Template           string    `json:"template"`
	Capacity           int       `json:"capacity"`
	MainModule         string    `json:"mainModule"`
	RedundantModule    string    `json:"redundantModule"`
	IOType             string    `json:"ioType"`
	MarshallingCabinet string    `json:"marshallingCabinet"`
	ControllerID       string    `json:"controllerId,omitempty"`
	SourceRow          int       `json:"sourceRow"`
	Channels           []Channel `json:"channels"`
}

type Channel struct {
	Channel   int    `json:"channel"`
	Tag       string `json:"tag"`
	Member    string `json:"member,omitempty"`
	Template  string `json:"template,omitempty"`
	SourceRow int    `json:"sourceRow"`
	Reserve   bool   `json:"reserve"`
}
