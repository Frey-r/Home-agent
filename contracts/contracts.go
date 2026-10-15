package contracts

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

//go:embed schemas/*.json
var SchemasFS embed.FS

// Categorías de entidades soportadas en M1.
const (
	CategoryLight            = "light"
	CategorySwitch           = "switch"
	CategoryCover            = "cover"
	CategoryLock             = "lock"
	CategoryAlarmControlPanel = "alarm_control_panel"
	CategoryClimate          = "climate"
	CategoryMediaPlayer      = "media_player"
	CategorySensor           = "sensor"
	CategoryBinarySensor     = "binary_sensor"
	CategoryScene            = "scene"
	CategoryScript           = "script"
	CategoryTodo             = "todo"
	CategoryWeather          = "weather"
)

// Acciones canónicas de entidades.
const (
	ActionSetPower            = "set_power"
	ActionSetBrightness       = "set_brightness"
	ActionSetColorTemp        = "set_color_temp"
	ActionSetColor            = "set_color"
	ActionOpen                = "open"
	ActionClose               = "close"
	ActionSetPosition         = "set_position"
	ActionStop                = "stop"
	ActionLock                = "lock"
	ActionUnlock              = "unlock"
	ActionUnlatch             = "unlatch"
	ActionArm                 = "arm"
	ActionDisarm              = "disarm"
	ActionSetHVACMode         = "set_hvac_mode"
	ActionSetTemperature      = "set_temperature"
	ActionSetTemperatureRange = "set_temperature_range"
	ActionSetFanMode          = "set_fan_mode"
	ActionSetPresetMode       = "set_preset_mode"
	ActionPlayback            = "playback"
	ActionSetVolume           = "set_volume"
	ActionSetMute             = "set_mute"
	ActionSelectSource        = "select_source"
	ActionActivate            = "activate"
	ActionRun                 = "run"
	ActionAddItem             = "add_item"
	ActionCompleteItem        = "complete_item"
	ActionRemoveItem          = "remove_item"
	ActionListItems           = "list_items"
	ActionGetForecast         = "get_forecast"
)

// Kind representa el tipo de capacidad (command o query).
type Kind string

const (
	KindCommand Kind = "command"
	KindQuery   Kind = "query"
)

// Risk representa el nivel de riesgo de una capacidad o entidad.
type Risk string

const (
	RiskLow      Risk = "low"
	RiskMedium   Risk = "medium"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

// Rank devuelve el orden numérico del riesgo para comparaciones.
func (r Risk) Rank() int {
	switch r {
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	case RiskCritical:
		return 4
	default:
		return 0
	}
}

// MaxRisk devuelve el mayor entre dos niveles de riesgo.
func MaxRisk(a, b Risk) Risk {
	if a.Rank() >= b.Rank() {
		return a
	}
	return b
}

// Source define el origen de una entidad en el ecosistema.
type Source struct {
	Kind     string  `json:"kind"`
	EntityID string  `json:"entity_id"`
	DeviceID *string `json:"device_id,omitempty"`
}

// Area representa la ubicación de una entidad en el hogar.
type Area struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Capability describe una acción ejecutable sobre una entidad.
type Capability struct {
	Action       string         `json:"action"`
	Kind         Kind           `json:"kind"`
	Risk         Risk           `json:"risk"`
	RequiresCode bool           `json:"requires_code"`
	Params       map[string]any `json:"params"`
}

// Entity representa una entidad normalizada v1 según el contrato unificado.
type Entity struct {
	ID           string         `json:"id"`
	Source       Source         `json:"source"`
	Name         string         `json:"name"`
	Aliases      []string       `json:"aliases"`
	Category     string         `json:"category"`
	DeviceClass  *string        `json:"device_class,omitempty"`
	Area         *Area          `json:"area,omitempty"`
	Exposed      bool           `json:"exposed"`
	Available    *bool          `json:"available,omitempty"`
	Risk         Risk           `json:"risk"`
	LastChanged  string         `json:"last_changed,omitempty"`
	State        map[string]any `json:"state"`
	Capabilities []Capability   `json:"capabilities"`
}

// Validator compila y valida documentos contra los esquemas embebidos.
type Validator struct {
	entityResolved *jsonschema.Resolved
	llmResolved    *jsonschema.Resolved
}

// NewValidator compila los esquemas entity.v1.json y llm.v1.json.
func NewValidator() (*Validator, error) {
	dataEntity, err := SchemasFS.ReadFile("schemas/entity.v1.json")
	if err != nil {
		return nil, fmt.Errorf("error al leer schemas/entity.v1.json: %w", err)
	}

	var sEntity jsonschema.Schema
	if err := json.Unmarshal(dataEntity, &sEntity); err != nil {
		return nil, fmt.Errorf("error al deserializar entity.v1.json: %w", err)
	}

	resolvedEntity, err := sEntity.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("error al resolver entity.v1.json: %w", err)
	}

	dataLLM, err := SchemasFS.ReadFile("schemas/llm.v1.json")
	if err != nil {
		return nil, fmt.Errorf("error al leer schemas/llm.v1.json: %w", err)
	}

	var sLLM jsonschema.Schema
	if err := json.Unmarshal(dataLLM, &sLLM); err != nil {
		return nil, fmt.Errorf("error al deserializar llm.v1.json: %w", err)
	}

	resolvedLLM, err := sLLM.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("error al resolver llm.v1.json: %w", err)
	}

	return &Validator{
		entityResolved: resolvedEntity,
		llmResolved:    resolvedLLM,
	}, nil
}

// ValidateEntity valida un payload JSON de entidad contra el esquema v1.
func (v *Validator) ValidateEntity(doc []byte) error {
	var instance any
	if err := json.Unmarshal(doc, &instance); err != nil {
		return fmt.Errorf("JSON inválido: %w", err)
	}

	if err := v.entityResolved.Validate(instance); err != nil {
		return fmt.Errorf("error de validación de entidad: %w", err)
	}
	return nil
}

// ValidateLLM valida un documento JSON (solicitud o respuesta) contra el esquema llm.v1.json.
func (v *Validator) ValidateLLM(doc []byte) error {
	var instance any
	if err := json.Unmarshal(doc, &instance); err != nil {
		return fmt.Errorf("JSON inválido: %w", err)
	}

	if err := v.llmResolved.Validate(instance); err != nil {
		return fmt.Errorf("error de validación de esquema LLM: %w", err)
	}
	return nil
}

// ValidateLLMResponse serializa y valida una respuesta contra llm.v1.json.
func (v *Validator) ValidateLLMResponse(resp LLMResponse) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("error al serializar respuesta LLM: %w", err)
	}
	return v.ValidateLLM(b)
}

// ValidateLLMRequest serializa y valida una solicitud contra llm.v1.json.
func (v *Validator) ValidateLLMRequest(req LLMRequest) error {
	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("error al serializar solicitud LLM: %w", err)
	}
	return v.ValidateLLM(b)
}

// Validate serializa y valida una entidad contra el esquema v1.
func (v *Validator) Validate(ent *Entity) error {
	b, err := json.Marshal(ent)
	if err != nil {
		return fmt.Errorf("error al serializar entidad: %w", err)
	}
	return v.ValidateEntity(b)
}

// ValidateParams valida un conjunto de parámetros contra el esquema de la capacidad.
func ValidateParams(cap Capability, params map[string]any) error {
	if cap.Params == nil {
		return nil
	}

	data, err := json.Marshal(cap.Params)
	if err != nil {
		return fmt.Errorf("error al serializar esquema de params: %w", err)
	}

	var s jsonschema.Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("error al parsear esquema de params: %w", err)
	}

	resolved, err := s.Resolve(nil)
	if err != nil {
		return fmt.Errorf("error al resolver esquema de params: %w", err)
	}

	if err := resolved.Validate(params); err != nil {
		return fmt.Errorf("parámetros inválidos para acción %s: %w", cap.Action, err)
	}
	return nil
}
