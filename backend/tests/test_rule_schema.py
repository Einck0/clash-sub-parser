from app.schemas.rule import RuleCreate


def test_rule_type_is_normalized_to_uppercase() -> None:
    a = RuleCreate(type="process-name", value="x", proxy="DIRECT")
    assert a.type == "PROCESS-NAME"

    b = RuleCreate(type="domain-suffix", value="example.com", proxy="DIRECT")
    assert b.type == "DOMAIN-SUFFIX"


def test_rule_supports_optional_name() -> None:
    item = RuleCreate(name="Google 直连", type="domain", value="google.com", proxy="DIRECT")
    assert item.name == "Google 直连"
    assert item.type == "DOMAIN"
