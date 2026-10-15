> **DOCUMENTO HISTÓRICO.** Ya no es la arquitectura vigente; la reemplaza [docs/arquitectura.md](docs/arquitectura.md).
>
> Las decisiones que motivan el cambio son:
> - [ADR-0003](docs/adr/0003-construir-sobre-home-assistant.md): construir sobre Home Assistant, sin worker Python con `uv`, MQTT, mDNS ni OTA propios.
> - [ADR-0004](docs/adr/0004-cerebro-edge-nube-cloudflare-relay-durable-objects.md): relay con Durable Objects en lugar de un túnel por hogar.
>
> Se conserva para mantener el contexto histórico.

Documento de Arquitectura del Sistema: Hub de Domótica Comercial con IA (Go + Python/uv)Este documento define la arquitectura técnica para un producto comercial de automatización del hogar impulsado por Inteligencia Artificial, diseñado para ejecutarse en hardware embebido (ARM64 / Raspberry Pi) sin depender de contenedores pesados en el dispositivo final.1. Visión General de la ArquitecturaEl sistema opera bajo un patrón Core Maestro en Go + Worker Dinámico en Python, donde Go controla el ciclo de vida del dispositivo, las comunicaciones seguras, el motor de IA y la interfaz de usuario, mientras que Python actúa como un subproceso aislado de traducción de hardware gestionado por uv.Plaintext                       [ NUBE DE CONTROL (Tu SaaS) ]
                        • API de Aprovisionamiento
                        • Creación dinámica de Cloudflare Tunnels
                        • Servidor de Actualizaciones OTA y Plugins
                                     │
                                     ▼ (Túnel Zero-Trust TLS)
┌─────────────────────────────────────────────────────────────────────────┐
│ DISPOSITIVO EDGE (Raspberry Pi / Linux ARM64)                           │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │ 1. BINARIO PRINCIPAL EN GO (edge-core)                            │  │
│  │  ├── Supervisor de Procesos: Gestiona cloudflared y uv (Python)   │  │
│  │  ├── Escáner Ligero: mDNS (Zeroconf) + SSDP/UPnP                  │  │
│  │  ├── AI Harness: Servidor MCP, Context Builder, Guardrails y LLM  │  │
│  │  ├── Broker de Eventos: Mochi-MQTT embebido en memoria            │  │
│  │  ├── Persistencia: SQLite (Modo WAL)                              │  │
│  │  └── UI Server: PWA embebida (//go:embed) + WebSockets            │  │
│  └───────────────┬───────────────────────────────────┬───────────────┘  │
│                  │ (Subproceso)                      │ (IPC: MQTT/UDS)  │
│                  ▼                                   ▼                  │
│  ┌──────────────────────────────┐    ┌───────────────────────────────┐  │
│  │ 2. AGENTE CLOUDFLARE         │    │ 3. ENTORNO PYTHON (uv)        │  │
│  │  • Binario `cloudflared`     │    │  • Intérprete aislado         │  │
│  │  • Token inyectado en RAM    │    │  • Instalación bajo demanda   │  │
│  └──────────────────────────────┘    │  • Drivers IoT (LG, Samsung,  │  │
│                                      │    Tuya, Matter, Shelly, BLE) │  │
│                                      └───────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
2. Componentes del Sistema y ResponsabilidadesComponenteTecnología / Librerías ClaveResponsabilidad PrincipalOrquestador Edge (edge-core)Go 1.23+Punto de entrada único (systemd). Administra la red, la base de datos, el motor de IA y vigila los subprocesos.Gestor de Entorno Pythonuv (Astral) (Binario estático ARM64)Crea el entorno virtual aislado, instala librerías de electrodomésticos en caliente en milisegundos y ejecuta el worker de Python.Bus de Comunicación Internamochi-mqtt/server/v2 o gRPC (Unix Socket)Comunica el núcleo de Go con los drivers de Python en tiempo real con latencia sub-milisegundo sin exponer puertos a la red.Motor de AutodescubrimientoGo (grandcat/zeroconf) + Python (netdisco / bleak)Detecta firmas de electrodomésticos en la LAN/Bluetooth e identifica qué librería de Python se necesita para controlarlos.AI HarnessGo (mark3labs/mcp-go + SDKs de LLM)Construye el contexto de la casa, expone los dispositivos como herramientas (Tools), aplica reglas de seguridad (Guardrails) y ejecuta acciones.Acceso Remoto Autoconfiguradocloudflared + cloudflare-go (en la nube)Establece un túnel saliente hacia Cloudflare sin abrir puertos en el router del cliente, creando un subdominio único por hogar.Almacenamiento Localglebarez/sqlite (Pure Go SQLite)Guarda el registro de dispositivos, credenciales locales encriptadas, estados históricos y métricas de consumo eléctrico.3. Flujos Operativos Clave3.1. Flujo de Detección e Instalación Dinámica de Drivers (Go + uv)Para mantener el dispositivo rápido y ligero, los drivers pesados solo se descargan cuando el electrodoméstico existe en la casa del usuario:Detección: El módulo de escaneo en Go escucha la red local vía mDNS/SSDP y detecta un anuncio: _webostv._tcp.local. en la IP 192.168.1.55.Resolución de Catálogo: Go consulta su catálogo interno (un archivo manifest.json) y determina que _webostv._tcp requiere el paquete de Python aiopylgtv==0.4.1.Inyección con uv: Go ejecuta en segundo plano:Bash/opt/domotica/bin/uv pip install --python /opt/domotica/venv aiopylgtv==0.4.1
Activación del Driver: Go envía un mensaje por el bus interno al worker de Python: {"action": "load_driver", "module": "lg_webos", "ip": "192.168.1.55"}.Carga Dinámica: El proceso de Python importa el módulo en caliente con importlib.import_module(), inicia el emparejamiento con el televisor y publica sus capacidades hacia Go.3.2. Flujo del AI Harness (Del Lenguaje Natural al Electrodoméstico)Cuando el usuario escribe o dice: "Voy a ver una película, prepara la sala":Inyección de Contexto Dinámico: El Harness en Go consulta en memoria el estado actual de los dispositivos de la habitación sala y genera un resumen estructurado (JSON compacto) con las capacidades disponibles.Evaluación con Tool Calling (MCP): Go envía la petición al LLM junto con las herramientas habilitadas: set_light_brightness, turn_on_tv, set_ac_temperature.Filtro de Guardrails (Seguridad en Go):El LLM devuelve las llamadas a herramientas.Go valida que los parámetros estén dentro de rangos seguros (ej. que el aire acondicionado no se configure a temperaturas extremas o que no se accione un dispositivo crítico bloqueado).Ejecución Asíncrona: Go dispara las órdenes hacia el worker de Python, quien ejecuta los comandos nativos sobre cada electrodoméstico en paralelo y confirma el cambio de estado.3.3. Flujo de Acceso Remoto "Zero-Config" (Cloudflare Tunnel)El usuario pulsa "Activar acceso fuera de casa" en la aplicación.El binario de Go envía una petición autenticada a tu servidor central ([api.tudominio.com/v1/tunnels/provision](https://api.tudominio.com/v1/tunnels/provision)).Tu servidor central usa la API de Cloudflare para crear el túnel hogar-<uuid> y el registro DNS CNAME hogar-<uuid>.tudominio.com, devolviendo únicamente el TUNNEL_TOKEN al dispositivo.El supervisor de Go levanta el binario local cloudflared tunnel run --token <TUNNEL_TOKEN> apuntando exclusivamente al puerto local de la interfaz web de Go (127.0.0.1:8080).4. Estructura de Archivos en el Dispositivo (Linux ARM64)Así queda organizado el sistema en la Raspberry Pi en producción para garantizar que las actualizaciones OTA no rompan los datos del usuario:Plaintext/opt/domotica/
├── bin/
│   ├── edge-core          # Tu binario principal compilado en Go
│   ├── uv                 # Binario estático de Astral uv (ARM64)
│   └── cloudflared        # Binario oficial de Cloudflare (ARM64)
├── python-worker/
│   ├── pyproject.toml     # Definición base del worker en Python
│   ├── main.py            # Demonio que escucha órdenes de Go (MQTT/Socket)
│   └── plugins/           # Adaptadores modulares por marca (lg.py, tuya.py, etc.)
├── venv/                  # Entorno virtual gestionado automáticamente por `uv`
└── data/
    ├── home.db            # Base de datos SQLite (Estados, Usuarios, Historial)
    ├── device_catalog.json# Mapa de firmas mDNS/BLE -> Paquetes PyPI
    └── secrets.enc        # Tokens de Cloudflare y llaves de API encriptadas
5. Contrato de Datos Estándar (Go ↔ Python)Para que el AI Harness en Go no tenga que saber cómo funciona internamente cada marca en Python, todos los drivers de Python deben traducir los electrodomésticos a una estructura estándar (Entidad Normalizada):JSON{
  "device_id": "lg_tv_sala_01",
  "name": "TV Sala",
  "category": "media_player",
  "room": "sala",
  "driver": "aiopylgtv",
  "state": {
    "power": "on",
    "volume": 15,
    "muted": false,
    "source": "Netflix"
  },
  "capabilities": [
    {"action": "set_power", "params": {"state": ["on", "off"]}},
    {"action": "set_volume", "params": {"level": {"type": "int", "min": 0, "max": 100}}},
    {"action": "launch_app", "params": {"app_id": ["Netflix", "YouTube", "HDMI1"]}}
  ]
}
Ventaja clave: Ese bloque capabilities se convierte directamente y de forma automática en el esquema JSON de Tools (Herramientas) que el AI Harness de Go le entrega al LLM. Cada vez que Python descubre un electrodoméstico nuevo, la IA aprende a usarlo al instante sin tocar el código de Go.