from app.utils.capability_filter import is_node_capability_qualified


def test_capability_filter_speed():
    # 无要求时直接通过
    assert is_node_capability_qualified(None) is True
    assert is_node_capability_qualified({"status": "fail"}) is True

    # 测速要求
    assert is_node_capability_qualified(None, min_speed_mbps=5.0) is False
    assert is_node_capability_qualified({"status": "ok", "speed_mbps": 10.0}, min_speed_mbps=5.0) is True
    assert is_node_capability_qualified({"status": "ok", "speed_mbps": 3.0}, min_speed_mbps=5.0) is False


def test_capability_filter_media():
    probe_ok = {
        "status": "ok",
        "media": {
            "youtube": {"status": "ok", "region": "HK"},
            "netflix": {"status": "full", "region": "HK"},
            "chatgpt": {"status": "ok"},
            "gemini": {"status": "ok"},
            "meta_ai": {"status": "ok"},
        },
    }

    # 多选解锁：单一匹配
    assert is_node_capability_qualified(probe_ok, required_media=["chatgpt"]) is True
    assert is_node_capability_qualified(probe_ok, required_media=["gemini"]) is True
    assert is_node_capability_qualified(probe_ok, required_media=["chatgpt", "gemini"]) is True

    # 多选解锁：部分不满足
    assert is_node_capability_qualified(probe_ok, required_media=["chatgpt", "disney"]) is False

    # 失败探针
    assert is_node_capability_qualified({"status": "fail"}, required_media=["chatgpt"]) is False


def test_capability_filter_combined():
    probe = {
        "status": "ok",
        "speed_mbps": 25.5,
        "media": {
            "chatgpt": {"status": "ok"},
            "gemini": {"status": "ok"},
        },
    }
    # 同时满足测速门槛和解锁要求
    assert is_node_capability_qualified(probe, min_speed_mbps=20.0, required_media=["chatgpt", "gemini"]) is True
    assert is_node_capability_qualified(probe, min_speed_mbps=30.0, required_media=["chatgpt", "gemini"]) is False
    assert is_node_capability_qualified(probe, min_speed_mbps=20.0, required_media=["chatgpt", "netflix"]) is False
