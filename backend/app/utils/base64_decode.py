import base64


def try_base64_decode(content: str) -> str:
    candidate = "".join(content.strip().split())
    if not candidate:
        return content

    for decoder in (base64.b64decode, base64.urlsafe_b64decode):
        try:
            padded = candidate + "=" * (-len(candidate) % 4)
            decoded = decoder(padded)
            text = decoded.decode("utf-8")
            if text.strip():
                return text
        except Exception:
            continue
    return content
