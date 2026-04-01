# Retry transient API failures (unmarshal errors, empty responses)

## Overview
- orx не ретраит два типа transient ошибок от провайдеров через OpenRouter:
  1. **Truncated JSON** - провайдер обрывает ответ, `json.Unmarshal` падает с "unexpected end of JSON input"
  2. **Empty responses** - провайдер возвращает HTTP 200 с пустым `choices` массивом
- Оба сценария - временные сбои провайдера, которые проходят при повторном запросе
- Нужно сделать их retryable наравне с 5xx и network errors

## Context
- Файл: `internal/client/client.go` (496 строк)
- Тесты: `internal/client/client_test.go` (1120 строк)
- `parseResponse()` (строки 398-416) - точка обоих багов
- `isRetryable()` (строки 427-457) - классификация ошибок
- `retryableError` тип (строки 418-425) - маркер retryable ошибок
- Существующие тесты: `TestExecute_EmptyChoices` (259-278), `TestExecute_InvalidJSONResponse` (335-353)

## Development Approach
- **Testing approach**: TDD (тесты первыми)
- Маленькие изменения, каждый таск завершен и тесты зеленые
- **CRITICAL: каждый таск начинается с тестов, потом код**

## Testing Strategy
- Unit тесты с `testutil.NewTestServer()` и httptest
- Паттерн из существующих тестов: проверка retry через счетчик запросов к mock-серверу
- Образец: `TestExecute_Retry` (строки 106-149) - проверяет что сервер получает >1 запрос

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with ! prefix

## Implementation Steps

### Task 1: Add retry test for JSON unmarshal errors
- [x] write test `TestExecute_RetryOnUnmarshalError`: mock-сервер первые 2 раза отдает truncated JSON, 3-й раз - валидный ответ. Проверить что result.Status == "success" и attempts == 3
- [x] write test `TestExecute_RetryOnUnmarshalError_Exhausted`: mock-сервер всегда отдает truncated JSON. Проверить status == "error", error содержит "unmarshal", attempts == 3
- [x] run tests - новые тесты должны FAIL (red phase)

### Task 2: Implement retry for JSON unmarshal errors
- [x] в `parseResponse()` обернуть unmarshal error в `retryableError` вместо обычного `fmt.Errorf`
- [x] обновить существующий `TestExecute_InvalidJSONResponse` если нужно (теперь ожидается retry exhaustion вместо immediate fail)
- [x] run tests - все тесты должны PASS (green phase)

### Task 3: Add retry test for empty responses
- [ ] write test `TestExecute_RetryOnEmptyChoices`: mock-сервер первые 2 раза отдает пустой choices, 3-й раз - валидный. Проверить status == "success" и attempts == 3
- [ ] write test `TestExecute_RetryOnEmptyChoices_Exhausted`: mock-сервер всегда отдает пустой choices. Проверить status == "error", error содержит "no choices", attempts == 3
- [ ] run tests - новые тесты должны FAIL (red phase)

### Task 4: Implement retry for empty responses
- [ ] в `parseResponse()` обернуть "no choices" error в `retryableError`
- [ ] обновить существующий `TestExecute_EmptyChoices` если нужно (теперь ожидается retry exhaustion)
- [ ] run tests - все тесты должны PASS (green phase)

### Task 5: Verify acceptance criteria
- [ ] verify: truncated JSON ретраится до 3 раз
- [ ] verify: empty choices ретраится до 3 раз
- [ ] verify: оба типа ошибок при успешном retry возвращают success
- [ ] run full test suite (`go test ./...`)
- [ ] run linter (`golangci-lint run` или проектный линтер)

## Technical Details
- `parseResponse()` строки 400-401: заменить `fmt.Errorf("unmarshal response: %w", err)` на `&retryableError{statusCode: 0, body: ...}`
- `parseResponse()` строки 411-412: заменить `fmt.Errorf("no choices in response")` на `&retryableError{statusCode: 0, body: ...}`
- statusCode=0 для этих ошибок т.к. HTTP status был 200, ошибка в содержимом
- `isRetryable()` уже поддерживает `retryableError` через `errors.As()` - новый код не нужен
- retryDelay=5s, maxRetries=3 - существующие параметры, менять не надо

## Post-Completion
- Проверить на реальных запросах к OpenRouter с моделями которые ранее давали пустые ответы
