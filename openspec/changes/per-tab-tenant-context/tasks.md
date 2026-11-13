# Tasks: per-tab-tenant-context

## 1. Согласование интерфейса

- [x] 1.1 Показать в прототипе сценарии «переключение пространства без полного reload» и «шелл до резолва контекста (loading-стейт)», согласовать вид до реализации (design-system/prototype)

## 2. Сервер: резолв по заголовку

- [x] 2.1 Добавить `HeaderStrategy` в internal/auth (читает `X-Tenant-ID`, валидирует против активных членств, fallback на единственное членство при отсутствии заголовка); тесты resolve_strategy на валидный/чужой/отсутствующий заголовок
- [x] 2.2 Подключить `HeaderStrategy` в цепочку TenantResolver в app/app.go вместо session-стратегии; прогнать `go test ./internal/auth/...`
- [x] 2.3 Различить ответы «нет членств» и «чужой tenant» отдельным кодом ошибки в API (middleware/handlers); тест на оба случая

## 3. Сервер: удаление серверного состояния

- [x] 3.1 Упразднить `POST /api/v1/session/tenant` и `SetActiveTenant` в store/sessions; удалить из routes; прогнать тесты session/*
- [x] 3.2 Миграция: удалить колонку `auth_sessions.active_tenant_id`; обновить seed_demo.sql при необходимости; прогнать `go test ./internal/store/...`
- [x] 3.3 Снять `RequireMembershipMiddleware` со страниц шелла (шелл tenant-agnostic), оставить на API; ревизия web/shell, web/noaccess, web/openlink, web/invite на чтение tenant до JS; тесты web/handlers

## 4. Клиент: хранение и заголовок

- [x] 4.1 api-клиент web/static: читать контекст вкладки (sessionStorage → localStorage → /session/tenants), проставлять `X-Tenant-ID` во все API-запросы; проверить в devtools, что заголовок уходит
- [x] 4.2 SidebarTenant: переключение пишет sessionStorage+localStorage и обновляет данные без POST /session/tenant; проверить, что вторая вкладка не меняет контекст первой
- [x] 4.3 Клиентский no-access: пустой список членств → экран no-access; недоступное сохранённое значение → тихий перебор на доступное с обновлением хранилищ; проверить сценарий «отозван доступ»

## 5. Кеширование и финал

- [x] 5.1 Добавить `Vary: X-Tenant-ID` в setAPICacheControl для tenant-scoped ответов; тест в internal/http/handlers/api/v1/cache_test.go
- [x] 5.2 Обновить карточки/прототип дизайн-системы (переключатель пространств), пересобрать `./design-system/build.sh` и `./design-system/prototype/build.sh`
- [ ] 5.3 Обновить README (если описан переключатель пространств / сессионная модель), прогнать полный `go test ./...`
- [x] 5.4 E2E-проверка: две вкладки в разных пространствах, запросы каждой в свой tenant; перезапуск браузера восстанавливает последний выбор
