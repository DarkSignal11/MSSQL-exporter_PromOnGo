package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/microsoft/go-mssqldb"
	"github.com/natefinch/lumberjack"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Пути к файлам
var (
	baseDir, _  = filepath.Abs(filepath.Dir(os.Args[0]))
	logFilePath = filepath.Join(baseDir, "logs", "mssql_exporter.log")
	confDir     = filepath.Join(baseDir, "conf")
	pidFilePath = filepath.Join(baseDir, "mssql_exporter.pid")
	reg         = prometheus.NewRegistry()
)

// Глобальные переменные
var (
	metrics = map[string]*prometheus.GaugeVec{}
	dbPool  = map[string]*sql.DB{}
)

// Настройка логирования с ротацией
func setupLogging() {
	os.MkdirAll(filepath.Dir(logFilePath), 0755)
	log.SetOutput(&lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    10, // MB
		MaxBackups: 5,  // Количество архивов
		MaxAge:     7,  // Дней хранения
		Compress:   true,
	})
	log.Println("📄 Логирование настроено:", logFilePath)
}

// Читаем все .env файлы
func loadConfigs() ([]map[string]string, error) {
	var configs []map[string]string
	os.MkdirAll(confDir, 0755)

	envFiles, err := filepath.Glob(filepath.Join(confDir, "*.env"))
	if err != nil {
		return nil, err
	}

	for _, file := range envFiles {
		env, err := godotenv.Read(file)
		if err != nil {
			log.Println("❌ Ошибка чтения файла:", file, err)
			continue
		}
		configs = append(configs, env)
		log.Println("✅ Загружен конфиг:", file)
	}
	return configs, nil
}

// Подключение к MSSQL
func getDBConn(config map[string]string) (*sql.DB, error) {
	dbKey := fmt.Sprintf("%s:%s:%s", config["SERVER"], config["PORT"], config["DATABASE"])
	if db, exists := dbPool[dbKey]; exists {
		return db, nil
	}

	connStr := fmt.Sprintf("server=%s;user id=%s;password=%s;port=%s;database=%s",
		config["SERVER"], config["USER"], config["PASSWORD"], config["PORT"], config["DATABASE"])

	log.Println("🔗 Подключение к MSSQL:", dbKey)
	db, err := sql.Open("sqlserver", connStr)
	if err != nil {
		log.Println("❌ Ошибка подключения:", err)
		return nil, err
	}
	dbPool[dbKey] = db
	return db, nil
}

// Сбор метрик с очисткой устаревших значений
func collectMetrics(config map[string]string) {
	metricName := config["PROM_METRIC_NAME"]
	valueColumn := config["METRIC_VALUE_COLUMN"]
	valueType := strings.ToLower(config["METRIC_VALUE_TYPE"])
	if valueType == "" {
		valueType = "float"
	}

	sqlQuery := config["SQL_QUERY"]
	log.Println("📌 Запрос для метрики", metricName, "=>", sqlQuery)
	db, err := getDBConn(config)
	if err != nil {
		log.Println("❌ Ошибка подключения:", err)
		return
	}

	rows, err := db.Query(sqlQuery)
	if err != nil {
		log.Println("❌ Ошибка SQL-запроса:", err)
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		log.Println("❌ Ошибка получения колонок:", err)
		return
	}

	valueIndex := -1
	for i, column := range columns {
		if column == valueColumn {
			valueIndex = i
			break
		}
	}
	if valueIndex == -1 {
		log.Println("❌ Колонка для значений не найдена:", valueColumn)
		return
	}

	if _, exists := metrics[metricName]; !exists {
		metrics[metricName] = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: metricName, Help: "Dynamic SQL metrics"},
			append(columns[:valueIndex], columns[valueIndex+1:]...),
		)
		reg.MustRegister(metrics[metricName])
	}

	// Очистка старых значений
	metrics[metricName].Reset()

	for rows.Next() {
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			log.Println("❌ Ошибка чтения строки:", err)
			continue
		}

		labels := make([]string, 0)
		for i := range columns {
			if i != valueIndex {
				labels = append(labels, fmt.Sprintf("%v", values[i]))
			}
		}

		var value float64
		if valueType == "string" {
			value = 1
		} else {
			value, _ = strconv.ParseFloat(fmt.Sprintf("%v", values[valueIndex]), 64)
		}

		log.Println("📡 Обновляем метрику:", metricName, labels, "=>", value)
		metrics[metricName].WithLabelValues(labels...).Set(value)
	}
}

// Основной процесс экспорта
func startExporter() {
	setupLogging()

	configs, err := loadConfigs()
	if err != nil {
		log.Fatal("❌ Ошибка загрузки конфигов:", err)
	}

	// Записываем PID
	pid := os.Getpid()
	os.WriteFile(pidFilePath, []byte(strconv.Itoa(pid)), 0644)
	log.Println("🔹 Запущен экспортёр (PID):", pid)

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	log.Println("🚀 MSSQL Exporter запущен на :9399")

	go func() {
		for {
			for _, config := range configs {
				collectMetrics(config)
				interval, err := strconv.Atoi(config["QUERY_INTERVAL"])
				if err != nil || interval <= 0 {
					interval = 30
				}
				time.Sleep(time.Duration(interval) * time.Second)
			}
		}
	}()

	log.Fatal(http.ListenAndServe(":9399", nil))
}

// Остановка экспортера
func stopExporter() {
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		log.Println("⚠️ Экспортёр не запущен (PID-файл отсутствует)")
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		log.Println("❌ Ошибка чтения PID:", err)
		return
	}

	process, err := os.FindProcess(pid)
	if err == nil {
		err = process.Kill()
	}
	if err != nil {
		log.Println("❌ Ошибка остановки процесса:", err)
	} else {
		log.Println("✅ Экспортёр остановлен (PID:", pid, ")")
		os.Remove(pidFilePath)
	}
}

// Запуск скрипта
func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: ./mssql_exporter start|stop|restart")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "start":
		startExporter()
	case "stop":
		stopExporter()
	case "restart":
		stopExporter()
		time.Sleep(1 * time.Second)
		startExporter()
	default:
		fmt.Println("Неизвестная команда:", os.Args[1])
		os.Exit(1)
	}
}
