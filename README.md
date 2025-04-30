# MSSQL Exporter for Prometheus

## Описание

MSSQL Exporter собирает метрики из Microsoft SQL Server и передаёт их в Prometheus.
Поддерживает несколько конфигураций через `.env` файлы, кастомные SQL-запросы и настраиваемые метрики.

## Возможности

- Подключение к нескольким MSSQL-серверам
- Гибкие SQL-запросы для метрик
- Автоматическое обновление метрик без рестарта
- Ротация логов

## Установка

### 1. Сборка из исходников

```sh
cd mssql_exporter
go build -o mssql_exporter
```

### 2. Установка зависимостей

```sh
go get github.com/microsoft/go-mssqldb
go get github.com/prometheus/client_golang/prometheus
go get github.com/joho/godotenv
go get github.com/natefinch/lumberjack
```

## Настройка

### 1. Создание конфигурационных файлов

Конфиги хранятся в `conf/` и имеют формат `.env`. Можно создать несколько файлов для разных подключений.

Пример `conf/database1.env`:

```
SERVER=127.0.0.1
PORT=1433
DATABASE=metrics_db
USER=sa
PASSWORD=yourpassword
SQL_QUERY=SELECT name, COUNT(*) AS value FROM sys.databases GROUP BY name
PROM_METRIC_NAME=mssql_db_count
METRIC_VALUE_COLUMN=value
METRIC_VALUE_TYPE=int
```

ВАЖНО!!! 
PROM_METRIC_NAME - Если на уровне самого SQL передается значение как String или похожее, либо число с единицами измерения (25,4 кг, 83%, 578,5%),
то лучше сразу делать преобразование в число в самом SQL-запроса. Иначе будете по умолчанию получать 1 или 0.

### 2. Запуск экспортера

```sh
./mssql_exporter start
```

По умолчанию экспортер слушает на порту `9399`. Метрики доступны по адресу:

```
http://localhost:9399/metrics
```

### 3. Управление процессом

```sh
./mssql_exporter stop   # Остановка экспортера
./mssql_exporter restart # Перезапуск экспортера
```

## Интеграция с Prometheus

Добавьте в конфиг Prometheus:

```yaml
scrape_configs:
  - job_name: 'mssql_exporter'
    static_configs:
      - targets: ['localhost:9399']
```

## Логи и отладка

Логи пишутся в `logs/mssql_exporter.log` с ротацией.

```sh
tail -f logs/mssql_exporter.log
```

## Пример установки в автозапуск

sudo nano /etc/systemd/system/mssql_exporter.service

[Unit]
Description=MSSQL Exporter for Prometheus
After=network.target

[Service]
Type=simple
ExecStartPre=/bin/rm -f /opt/mssql_exporter/mssql_exporter.pid
ExecStart=/opt/mssql_exporter/mssql_exporter start
Restart=always
User=softuser
WorkingDirectory=/opt/mssql_exporter
StandardOutput=append:/opt/mssql_exporter/logs/mssql_exporter.log
StandardError=append:/opt/mssql_exporter/logs/mssql_exporter.log

[Install]
WantedBy=multi-user.target



sudo systemctl daemon-reload
sudo systemctl enable mssql_exporter
sudo systemctl start mssql_exporter
sudo systemctl status mssql_exporter


## Совместимость

Поддерживаемые версии MSSQL:

- Microsoft SQL Server 2008+
- Azure SQL Database
- Azure SQL Managed Instance

## TODO

- Автоматическая перезагрузка конфигов без рестарта
- Поддержка Kerberos-аутентификации (если потребуется)

---

Автор: Kazybek Mukhanov [DarkSignal]

