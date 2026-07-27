# KNS SSH

Веб-доступ к терминалу машины через центральный сервер: агент на целевой машине держит исходящее WebSocket-соединение к серверу, браузер (в т.ч. с телефона) подключается к серверу, логинится, видит список своих компьютеров и открывает SSH-консоль (xterm.js → WS → PTY/bash).

## Сборка

```bash
# 1. Фронтенд (обязательно перед сборкой Go — dist встраивается в бинарник)
cd frontend && npm install && npm run build && cd ..

# 2. Бинарник
go build -o kns-ssh .
```

## Запуск

### 1. Сервер

```bash
./kns-ssh serve --http=0.0.0.0:8090 --dir=pb_data
```

### 2. Создать пользователя и компьютер

```bash
./kns-ssh create-user --email=you@example.com --password=secret123
./kns-ssh create-agent --email=you@example.com --name=my-mac   # сгенерирует токен
```

На Railway (где нет shell) то же самое делается env-переменными при первом старте:

```
KNS_ADMIN_EMAIL=you@example.com
KNS_ADMIN_PASSWORD=secret123
KNS_AGENT_NAME=my-mac
KNS_AGENT_TOKEN=<длинная-случайная-строка>
```

Пользователь/агент создаются только если их ещё нет — переменные можно не убирать.

### 3. Агент на целевой машине

```bash
./kns-ssh agent -t <AGENT_TOKEN> -s wss://ssh.knswebworks.com
```

#### Автозапуск на macOS (launchd)

```bash
# подставь свой токен в launchd/com.knswebworks.kns-ssh-agent.plist (PASTE_AGENT_TOKEN_HERE)
cp launchd/com.knswebworks.kns-ssh-agent.plist ~/Library/LaunchAgents/
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.knswebworks.kns-ssh-agent.plist
```

Агент стартует при входе в систему и перезапускается при падении. Логи: `~/Library/Logs/kns-ssh-agent.err.log`. Остановить: `launchctl bootout gui/$(id -u)/com.knswebworks.kns-ssh-agent`.

### 4. Браузер / телефон

Открой `https://ssh.knswebworks.com` → войди по email/паролю → увидишь список своих компьютеров с индикатором online → клик → терминал.

## API

- `POST /api/collections/users/auth-with-password` — логин (PocketBase).
- `GET /api/agents/online` — список агентов текущего пользователя с флагом `online` (требует `Authorization: <pb token>`).
- `GET /api/ws/agent?token=...` — подключение агента (токен проверяется по БД).
- `GET /api/ws/client?session_id=...&agent_id=...` — подключение веб-клиента к агенту (400, если агент оффлайн).

## Деплой на Railway

Репозиторий собирается через `Dockerfile` (multi-stage: frontend → Go binary). Порт берётся из `$PORT` (Railway передаёт сам). Данные PocketBase пишутся в `/pb_data` — там смонтирован Railway Volume, пользователи и агенты переживают передеплои.

## Протестировано

- Полный сценарий: env-bootstrap → логин → список агентов → консоль → выполнение команды в bash.
- Негативные кейсы: список без авторизации → 401, агент с неверным токеном → 401, клиент к оффлайн-агенту → 400.
- Две параллельные сессии на одном агенте, сборка с `-race` — гонок нет.
- Терминалы: несколько сессий на агенте, reattach с replay scrollback после перезагрузки страницы, restart сессии, SIGINT (Ctrl+C), resize (stty size), TERM=xterm-256color для TUI-приложений (claude code и т.п.).

## Возможности терминала

- **Вкладки**: кнопка «+ New terminal» открывает несколько независимых shell-сессий на одном компьютере.
- **Персистентность**: shell живёт на агенте; перезагрузка страницы/возврат из списка — переподключение с replay последних ~256KB вывода. Закрытие вкладки крестиком убивает shell.
- **Restart**: кнопка ⟳ пересоздаёт shell текущей вкладки.
- **Клавиши**: Ctrl+C (без выделения — SIGINT, с выделением — копирование), Ctrl+Z/X/A/D/L/E/U/K/W/R, Ctrl+V / Cmd+V — вставка.
- **TUI**: агент стартует bash с TERM=xterm-256color и COLORTERM=truecolor, передаёт resize — полноэкранные приложения (vim, htop, claude code) работают.

## Известные ограничения прототипа

- Токен агента — единственный секрет агента; зная его, можно подключиться как агент или открыть к нему консоль. Держи токены длинными и случайными.
- WS endpoint клиента пока не проверяет принадлежность агента пользователю (только валидность токена).
- Сессии живут в памяти агента: перезапуск агента убивает все shell.
