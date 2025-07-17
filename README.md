# LinLang-Go 🧮

Un mini lenguaje de programación algebraico, diseñado para modelar subespacios vectoriales, transformaciones y relaciones algebraicas, escrito en Go.

## 📂 Estructura

- `core/` - Núcleo del lenguaje: espacios, vectores, transformaciones, condicionales.
- `parser/` - Parser simple que interpreta archivos `.lin`.
- `main.go` - Ejemplo principal de ejecución.
- `ejemplo.lin` - Ejemplo de código LinLang.
- `Dockerfile` - Contenedor listo para correr LinLang.
- `docker-compose.yml` - Ejecución sencilla vía Docker Compose.

## 🚀 Cómo ejecutar

### Requisitos:
- Docker y Docker Compose instalados.

### Pasos:
1. Clonar el repositorio:
```bash
git clone https://github.com/TU_USUARIO/linlang-go.git
cd linlang-go



📌 Roadmap
✅ Subespacios vectoriales
✅ Vectores y transformaciones
✅ Condicionales básicos
✅ Docker-ready
⬜️ Extensión a REST/gRPC
⬜️ Persistencia con Redis/Mongo
⬜️ Orquestación con Kubernetes