import logging
import json
import sys
from datetime import datetime, timezone


class JSONFormatter(logging.Formatter):
    def format(self, record):
        log_entry = {
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
        }
        if record.exc_info and record.exc_info[0]:
            log_entry["exception"] = self.formatException(record.exc_info)
        if hasattr(record, "request_id"):
            log_entry["request_id"] = record.request_id
        if hasattr(record, "method"):
            log_entry["method"] = record.method
        if hasattr(record, "path"):
            log_entry["path"] = record.path
        if hasattr(record, "status_code"):
            log_entry["status_code"] = record.status_code
        if hasattr(record, "duration_ms"):
            log_entry["duration_ms"] = record.duration_ms
        return json.dumps(log_entry, ensure_ascii=False)


class HumanFormatter(logging.Formatter):
    def format(self, record):
        ts = datetime.now().strftime("%H:%M:%S")
        msg = record.getMessage()
        if record.levelno >= logging.ERROR:
            return f"\033[31m{ts} [{record.levelname}] {msg}\033[0m"
        elif record.levelno >= logging.WARNING:
            return f"\033[33m{ts} [{record.levelname}] {msg}\033[0m"
        return f"{ts} [{record.levelname}] {msg}"


def setup_logging(json_mode: bool = False, level: str = "INFO"):
    root = logging.getLogger()
    root.setLevel(getattr(logging, level.upper(), logging.INFO))
    
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JSONFormatter() if json_mode else HumanFormatter())
    root.handlers = [handler]
    
    # Quiet noisy loggers
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("httpcore").setLevel(logging.WARNING)
    logging.getLogger("apscheduler").setLevel(logging.WARNING)
