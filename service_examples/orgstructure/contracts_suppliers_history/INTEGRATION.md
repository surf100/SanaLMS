# Интеграция: аудит изменений договоров

## Импорт

```go
import csh "encore.app/orgstructure/contracts_suppliers_history"
```

## Паттерн

1. Получить состояние **до** мутации
2. Выполнить мутацию
3. Получить состояние **после** мутации
4. Конвертировать обе строки через `csh.EntToContract()`
5. Вызвать `csh.InsertAuditRecord(ctx, id, opType, old, new)`

## Соответствие эндпоинтов

| Эндпоинт | Операция |
|---|---|
| `POST /suppliers/{id}/contracts` | `csh.OpCreate` |
| `PATCH /contracts-suppliers/id/{id}` | `csh.OpUpdate` |
| `POST /contracts-suppliers/id/{id}/amendment` | `csh.OpUpdate` |
| `POST /contracts-suppliers/id/{id}/upload-file` | `csh.OpUpdate` |
| `DELETE /contracts-suppliers/id/{id}` | `csh.OpDelete` |

## Пример: CREATE

Старого состояния нет — передаём `nil`.

```go
newContract := csh.EntToContract(insertedRow)
csh.InsertAuditRecord(ctx, newContract.ID, csh.OpCreate, nil, newContract)
```

## Пример: UPDATE

```go
oldContract := csh.EntToContract(rowBefore)
// ... мутация ...
newContract := csh.EntToContract(rowAfter)
csh.InsertAuditRecord(ctx, id, csh.OpUpdate, oldContract, newContract)
```

## Пример: DELETE

```go
oldContract := csh.EntToContract(rowBefore)
// ... soft delete ...
newContract := *oldContract
newContract.IsActive = false
csh.InsertAuditRecord(ctx, id, csh.OpDelete, oldContract, &newContract)
```

## Что записывается автоматически

- `changed_by` — из контекста авторизации (Keycloak ID)
- `snapshot` — полное состояние после мутации
- `diff` — только изменённые поля: `{ "field": { "old": X, "new": Y } }`

## Ответы эндпоинтов

### GET /contracts-suppliers/id/{id}/history

Успешный ответ (сортировка по `changed_at` DESC):

```json
{
    "records": [
        {
            "history_id": "33333333-3333-3333-3333-333333333333",
            "contract_id": "11111111-1111-1111-1111-111111111111",
            "operation_type": "UPDATE",
            "changed_at": "2026-04-20T17:50:24.925113+05:00",
            "changed_by": "00000000-0000-0000-0000-000000000001",
            "snapshot": {
                "id": "11111111-1111-1111-1111-111111111111",
                "contract_number": "№123/2025/1",
                "amount": 1000000,
                "is_active": true,
                "vat_flag": true
            },
            "diff": {
                "amount": { "old": 800, "new": 1000000 },
                "vat_flag": { "old": false, "new": true }
            }
        }
    ],
    "total": 1
}
```

### GET /contracts-suppliers/id/{id}/validate

Договор валиден:

```json
{
    "result": {
        "is_valid": true
    }
}
```

Договор невалиден:

```json
{
    "result": {
        "is_valid": false,
        "errors": [
            "contract_number is required",
            "amount must be >= 0",
            "signed_date is required"
        ]
    }
}
```
