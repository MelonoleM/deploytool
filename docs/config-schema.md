# Configuration notes

`deploytool.yaml` follows the schema in the supplied development specification. All service names and runtime commands are declarative. `vars` can be referenced with `{{ .Vars.key }}`; an environment variable with the same name overrides that value before rendering.

The initial implementation supports `mkdir`, `extract` (`.tar.gz`), `copy_file`, `render_template`, `write_env`, `set_permission`, and `run_script` install actions. Each created file is recorded in local state with a reverse action; scripts without `undo_script` are explicitly marked as requiring manual cleanup rather than guessed at during uninstall.

State is stored atomically at `<install_root>/.deploytool/state.json`, along with operation history. This intentionally replaces the specification's suggested embedded database for the first dependency-minimal release; its small interface makes replacement with BoltDB or SQLite non-breaking.

## Service launch mode

Set `start_mode` per service to choose how `deploytool start` launches it:

```yaml
start: "./application --port 8080"
start_mode: background # default: returns after health verification; output is in .deploytool/logs/<service>.log
```

Use `start_mode: console` when diagnosing a single service from an operator terminal. It attaches stdout, stderr, and stdin to that terminal and intentionally blocks until the process exits; use Ctrl+C to stop it. Console mode should be invoked as `deploytool start service-name`, not from a Web UI or a multi-service installation, because it is interactive.

Update packages are tar.gz archives created by `deploytool pack`; they contain a generated `manifest.json`. The tool verifies every SHA-256 before copying files. The current package layout is relative to each selected service work directory.

## Java delivery shapes

The schema has no notion of a jar: `type: java` is a label, and the engine only copies the files declared in `install.steps` and runs the declared `start` command. A service therefore ships loose class files (`java -cp . ExampleServer`) or a packaged jar (`java -jar app.jar`) with the same configuration shape. The bundled example demonstrates both, as `java-api` and `java-jar-api`.
