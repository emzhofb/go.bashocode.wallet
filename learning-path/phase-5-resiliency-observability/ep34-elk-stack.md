# Episode 34: Centralized Log Aggregation (ELK Stack)

## 🎯 Tujuan
* Memahami tantangan debug log di multi-microservice (*log tracing isolation*).
* Menambahkan **Elasticsearch**, **Logstash**, dan **Kibana** (ELK Stack) ke Docker Compose.
* Mengirim structured JSON logs (slog/zap) dari seluruh container microservice ke Logstash.
* Melakukan visualisasi pencarian log terpadu di dashboard Kibana berdasarkan `correlation_id` atau `user_id`.

---

## 📐 Mengapa Butuh Centralized Log Aggregation?
Jika kita mendebug transaksi gagal di production microservices:
* Tanpa ELK: Kita harus SSH ke 5 server berbeda, mengetikkan `docker logs -f <service>` berkali-kali, dan mencocokkan jam manual. Ini melelahkan dan memakan waktu berjam-jam.
* Dengan ELK: Semua log JSON dikirim instan ke satu bank data terpusat (Elasticsearch). Kita cukup mengetikkan `correlation_id` di kotak pencarian Kibana, dan seluruh log yang dilalui transaksi tersebut dari Gateway hingga Database tampil berurutan dalam satu detik.

```
[ Gateway Log ]    ──┐
[ Wallet Log ]     ──┼─➔ [ Logstash (TCP) ] ➔ [ Elasticsearch ] ➔ [ Kibana UI ]
[ Auth Log ]       ──┘
```

---

## 📦 Langkah-langkah

### Step 1: Tambahkan ELK Stack ke Docker Compose
Buka file `docker-compose.yml` di folder root workspace, tambahkan service `elasticsearch`, `logstash`, dan `kibana`:

```yaml
# ... services mysql, redis, rabbitmq, mongodb, jaeger, prometheus, grafana ...
  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:8.12.0
    container_name: gowallet-elasticsearch
    environment:
      - discovery.type=single-node
      - xpack.security.enabled=false # Matikan security untuk simplifikasi lokal
      - "ES_JAVA_OPTS=-Xms512m -Xmx512m" # Batasi limit RAM agar tidak crash
    ports:
      - "9200:9200"
    restart: always

  logstash:
    image: docker.elastic.co/logstash/logstash:8.12.0
    container_name: gowallet-logstash
    volumes:
      - ./deployments/elk/logstash.conf:/usr/share/logstash/pipeline/logstash.conf
    ports:
      - "5044:5044" # Port input Beats
      - "5000:5000/tcp" # Port input TCP raw JSON
    environment:
      - "LS_JAVA_OPTS=-Xms256m -Xmx256m"
    depends_on:
      - elasticsearch
    restart: always

  kibana:
    image: docker.elastic.co/kibana/kibana:8.12.0
    container_name: gowallet-kibana
    ports:
      - "5601:5601" # Port Web UI Kibana
    depends_on:
      - elasticsearch
    restart: always
```

Buat file konfigurasi pipeline Logstash di `./deployments/elk/logstash.conf` di folder root workspace:

```logstash
input {
  tcp {
    port => 5000
    codec => json_lines # Menerima input stream per baris data JSON
  }
}

filter {
  # (Opsional) Lakukan parsing atau manipulasi data log di sini
}

output {
  elasticsearch {
    hosts => ["http://elasticsearch:9200"]
    index => "gowallet-logs-%{+YYYY.MM.dd}" # Nama index berformat tanggal harian
  }
  stdout {
    codec => rubydebug # Tampilkan debug log di terminal console logstash
  }
}
```

Jalankan container: `docker compose up -d`. 
* Kibana UI: `http://localhost:5601`

*(Catatan: Proses startup ELK Stack membutuhkan waktu sekitar 1 hingga 2 menit karena Elasticsearch mengindeks modul sistemnya).*

### Step 2: Konfigurasi Log Writer ke Logstash TCP di Go Code
Agar logs dikirim langsung ke port TCP Logstash `:5000`, kita buat custom writer di Go `internal/logger/logger.go` (pada masing-masing service):

```go
package logger

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
)

var Log *slog.Logger

func InitLogger(serviceName string, logstashAddr string) {
	var writer io.Writer

	// Koneksikan ke Logstash TCP
	conn, err := net.Dial("tcp", logstashAddr)
	if err == nil {
		// Gabungkan output log ke stdout konsol dan TCP logstash (MultiWriter)
		writer = io.MultiWriter(os.Stdout, conn)
	} else {
		// Jika logstash mati, log dikirim ke stdout saja agar aplikasi tetap berjalan
		writer = os.Stdout
		println("Warning: Could not connect to Logstash TCP, logging to stdout only: " + err.Error())
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Inject metadata nama service secara default di setiap baris log
	Log = slog.New(handler).With("service", serviceName)
	slog.SetDefault(Log)
}
```

### Step 3: Jalankan Logger Baru di Startup
Buka `cmd/main.go` di masing-masing service, inisialisasi logger dengan mempassing variabel environment:

```go
// Di main.go:
	logger.InitLogger("wallet-service", "localhost:5000")
```

### Step 4: Konfigurasi Index Pattern di Kibana
1. Buka Kibana UI (`http://localhost:5601`).
2. Masuk ke menu **Management** (ikon roda gigi) > **Stack Management** > **Kibana** > **Data Views** (atau **Index Patterns** di versi lama).
3. Klik **Create Data View**.
4. Set Name: `gowallet-*` (sesuai nama index pattern Logstash).
5. Set Timestamp field: `@timestamp`. Klik **Save data view**.
6. Klik menu **Discover** (ikon kompas di bar kiri), pilih data view `gowallet-*`, dan lakukan pencarian log real-time secara instan!

---

## ✅ Acceptance Criteria
* [ ] Container Elasticsearch, Logstash, dan Kibana berjalan lancar.
* [ ] Aplikasi Go berhasil mengirim log terstruktur ke Logstash TCP tanpa mengganggu jalannya aplikasi jika Logstash mati.
* [ ] Buka Kibana Discover, log JSON dari `api-gateway`, `auth-service`, dan `wallet-service` tampil lengkap dan searchable.

---

## 💡 Tips untuk Junior
* **RAM Optimization di Local:** ELK Stack sangat rakus memori RAM (bisa memakan 2GB hingga 3GB RAM secara default). Pembatasan limit memori di file Docker Compose (`ES_JAVA_OPTS=-Xms512m -Xmx512m`) adalah wajib hukumnya di komputer development lokal agar laptop Anda tidak lambat atau hang.

---

## 📚 Referensi Belajar
* [ELK Stack Complete Guide (Elastic)](https://www.elastic.co/elastic-stack)
* [Logstash TCP Input plugin documentation](https://www.elastic.co/guide/en/logstash/current/plugins-inputs-tcp.html)
