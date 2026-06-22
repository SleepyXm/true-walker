# True-Walker
 
**True-Walker** reads your codebase like a book — not by what things are named, but by what they do. It walks every file, extracts behavioral primitives like return shapes, route patterns, import usage, and type structure, then assigns semantic meaning from the ground up.
 
Ask it where authentication happens, how a profile is retrieved, or what owns the database layer — it tells you.
 
Part of the **InfraMap** family.
 
---
 
## How it works
 
Most code analysis tools tell you what exists. True-Walker tells you what things *do*.
 
It walks a codebase language by language, spawning a worker per language group. Each worker reads only the rules relevant to its language, then extracts behavioral signals from every file it processes:
 
- **Return shapes** — does this function return data, an error, a boolean, a message, or a combination?
- **Route patterns** — what HTTP routes does this file expose, what methods, what paths?
- **Import usage** — what does this file actually use from what it imports, and where?
- **Type structure** — what fields do these types carry, what do they extend?
- **Function signatures** — what goes in, what comes out?
From those signals it builds a behaviour model of the codebase. A function that returns `data+error` and sits behind a POST route is probably a write handler. A file that imports a database driver, defines column-annotated structs, and returns records is probably the persistence layer. The names don't matter — the behavior does.
 
---
 
## Supported languages
 
Go, Python, Rust, JavaScript, TypeScript, TSX, Ruby, Java, C, C++, Kotlin, PHP
 
---
 
## Project structure
 
```
yamls/
  routes.yml        # HTTP route and prefix patterns
  imports.yml       # Import statement patterns per language
  functions.yml     # Function definition patterns
  classes.yml         # Class, struct, and field patterns
  controls.yml  # Loop, assignment, and return patterns
```
 
Rules are plain YAML — deterministic, readable, and extensible without touching the engine. Each file owns one concern. Each worker loads only what it needs.
 
---
 
## Getting started
 
**Requirements:** Go 1.21+
 
```bash
git clone https://github.com/SleepyXm/true-walker
cd true-walker
go build ./...
```
 
**Run against a codebase:**
 
```bash
go run . --target /path/to/your/project --rules ./yamls
```
 
**Output** is a JSON snapshot per file containing extracted functions, imports, classes, routes, and return definitions.
 
---
 
## Extending the rules
 
Each YAML file follows the same structure — a list of named patterns with optional language scoping.
 
```yaml
function_rules:
  - name: my-custom-rule
    pattern: 'func\s+(?P<function>\w+)\s*\('
    language: .go
```
 
Named capture groups are the contract: `function`, `path`, `method`, `import`, `class`, `field`, `value` etc. A rule with no `language` field matches across all languages.
 
Add a rule, rerun — no recompilation needed.
 
---
 
## Querying
 
True-Walker produces a semantic map, not just a symbol index. The output is designed to be queried by humans and LLMs alike using natural language boundaries:
 
> *Where does authentication happen?*
> *What handles profile picture retrieval?*
> *What functions write to the database?*
 
Query support is part of the broader InfraMap platform.
 
---
 
## Licensing
 
True-Walker is dual licensed.
 
**GNU GPL v3** — free to use for open source projects. Any software built on top of True-Walker must also be released under GPL v3. Full license text in [`LICENSE`](./LICENSE).
 
**Commercial license** — for teams and companies that need to embed True-Walker in proprietary products, internal tooling, or closed-source systems without the GPL v3 open source requirement. Contact [license@inframap.io](mailto:license@inframap.io) for terms.
 
If you are unsure which license applies to your use case, the commercial license is the right choice.
 
---
 
## InfraMap
 
True-Walker is one part of InfraMap — a suite of tools for making large codebases legible. InfraMap products share a common extraction layer and a common query interface, so the same question works whether you're asking about a monolith, a set of microservices, or a mixed-language platform.
 
