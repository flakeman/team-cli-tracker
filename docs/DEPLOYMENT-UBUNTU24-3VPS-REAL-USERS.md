# Deployment: Ubuntu 24, 3 VPS, Real Users

## RU

Этот runbook описывает production-like развёртывание `team-cli-tracker` на 3 VPS и подключение реальных пользователей через issuer/policy.

## 0) Термины
- VPS/Node: серверный узел кластера (в вашем случае 3).
- Real user: человек с отдельным `user_id`, ролью и token.
- Simulated actor: технический актор в тесте (не реальный человек).

Количество VPS не ограничивает количество пользователей.

## 1) Предпосылки
- Ubuntu 24 на 3 VPS.
- Открыты сетевые порты для node API/peer communication.
- Работает issuer/policy endpoint для authn/authz.
- Локальная машина с Go для сборки бинарника.

## 2) Сборка бинарника (локально)
```bash
cd team-cli-tracker
go test ./...
GOOS=linux GOARCH=amd64 go build -o node ./cmd/node
```

## 3) Доставка бинарника на все VPS
Пример для вашего хоста и портов:
```bash
scp -P 22221 ./node vova@test.abuztech.ru:/home/vova/tct/node
scp -P 22222 ./node vova@test.abuztech.ru:/home/vova/tct/node
scp -P 22223 ./node vova@test.abuztech.ru:/home/vova/tct/node
```

На каждом VPS:
```bash
chmod +x ~/tct/node
mkdir -p ~/tct/data ~/tct/log
```

## 4) Быстрый запуск в screen (операционный smoke)
На каждом VPS:
```bash
screen -S tct_node -dm bash -lc '~/tct/node serve --project-id OPS --listen :4101 --data-dir ~/tct/data --secure-mode-required=true >> ~/tct/log/node.log 2>&1'
screen -ls
```

Проверка:
```bash
tail -n 50 ~/tct/log/node.log
```

Остановить:
```bash
screen -S tct_node -X quit
```

## 5) Устойчивый запуск через systemd (рекомендуется)
`/etc/systemd/system/team-cli-tracker.service`:
```ini
[Unit]
Description=team-cli-tracker node
After=network-online.target
Wants=network-online.target

[Service]
User=vova
WorkingDirectory=/home/vova/tct
ExecStart=/home/vova/tct/node serve --project-id OPS --listen :4101 --data-dir /home/vova/tct/data --secure-mode-required=true
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

Применить:
```bash
sudo systemctl daemon-reload
sudo systemctl enable team-cli-tracker
sudo systemctl restart team-cli-tracker
sudo systemctl status team-cli-tracker --no-pager
```

Логи:
```bash
journalctl -u team-cli-tracker -f
```

## 6) Онбординг реальных пользователей
Для каждого человека:
1. Создать identity в issuer (`user_id`).
2. Назначить роль (`admin`, `lead`, `dev`, `qa`, `viewer`).
3. Выпустить token с ограниченным TTL.
4. Передать token безопасно (секрет-хранилище, не chat/shell history).

Рекомендуемая минимальная матрица:
- `admin1` -> `admin`
- `lead1` -> `lead`
- `dev1` -> `dev`
- `qa1` -> `qa`
- `viewer1` -> `viewer`

## 7) Подключение реального пользователя к доске
Live mode:
```bash
team-cli-tracker board watch \
  --server http://<node-host>:4101 \
  --project-id OPS \
  --user-id <user_id> \
  --auth-token <token> \
  --view all
```

Interactive mode:
```bash
team-cli-tracker board watch \
  --server http://<node-host>:4101 \
  --project-id OPS \
  --user-id <user_id> \
  --auth-token <token> \
  --interactive \
  --interactive-refresh 2s
```

Команды внутри interactive:
- `help`
- `create`
- `move`
- `comment`
- `view`
- `counts`
- `table`
- `pause`
- `resume`
- `quit`

## 8) Проверка прав (allow/deny)
Позитивные кейсы:
- `lead/dev` могут создавать и двигать задачи в пределах policy.
- `qa` может переводить задачи по разрешённым переходам.

Негативные кейсы:
- `viewer` не может `create/move/comment`.
- Пользователь без роли не может мутировать данные.

## 9) Аудит-доказательство реальных пользователей
После теста:
```bash
team-cli-tracker audit export --all --format jsonl
team-cli-tracker audit export --from 2026-02-13T00:00:00Z --to 2026-02-13T23:59:59Z --user admin1,lead1,dev1,qa1,viewer1 --format csv
team-cli-tracker audit verify-integrity
```

Критерий:
- В экспорте видны действия с реальными `user_id`.
- Нет ложных "успешных" мутаций у запрещённых ролей.
- Integrity check проходит.

## 10) Операционный чеклист
- `secure-mode-required=true` в production.
- UTC на всех VPS.
- Ротация токенов и секретов.
- Регулярный экспорт аудита.
- В smoke-отчёте всегда разделять:
  - simulated actors,
  - real users (token-based).
