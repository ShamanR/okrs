## Repository navigation

Use `rg` or `git grep` for repository-wide search.
Prefer `rg` over `grep`.

Use Go tooling for semantic checks:

- `go list ./...`
- `go test ./...`
- `go vet ./...`

Do not assume the IDE provides a complete AST or semantic index.
Verify usages and dependencies from the actual repository.

## Сначала прочитай и используй как source of truth

- `README-specs.md`
- `specs/010-architecture-constraints.md`
- `specs/020-domain-model.md`
- `specs/030-user-flows.md`
- `specs/040-api-contract.md`
- `specs/050-permissions-and-lifecycle.md`

## Перед началом реализации

1. Кратко перечисли mismatch между текущим кодом и релевантными specs, если он есть.
2. Если specs нужно обновить для этой задачи, сделай это в том же change set.
3. Не меняй unrelated specs.
4. Поддерживай тесты в актуальном состоянии, покрывая новый функционал тестами
5. Do not mention Claude, AI, assistants, agents, or generated-by attribution in commit messages, PR titles, PR descriptions, changelogs, docs, or comments unless explicitly requested.
6. Follow clean architecture and layered architecture principles. Do not leak abstractions across layers.
7. Поддерживай seed demo в актуальном состоянии, если меняется структура таблиц
8. Dont make any git commits, i will do it myself
9. При разработке - убеждайся что код работает эффективно, ты не избегаешь запросы в базу данных в цикле.
10. Ты рассматриваешь возможность кеширования данных в inmemory или браузере (но учитываешь - что приложение будет развернуто в K8S среде на много инстансов). При этом понимаешь - какие пользовательские ожидания от консистентности
11. пиши дизайн-доки и спеки на русском
12. создавай констстентный интерфейс и консистентные элементы управления. стермись переиспольовать frontend компоненты
