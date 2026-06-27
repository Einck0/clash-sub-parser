from fastapi import HTTPException, status


class AppError(HTTPException):
    """Standard application error with code and detail."""
    def __init__(self, code: str, message: str, detail: str = "", status_code: int = 400):
        super().__init__(status_code=status_code, detail={"code": code, "message": message, "detail": detail})
        self.code = code


class NotFoundError(AppError):
    def __init__(self, resource: str, resource_id: int | str = ""):
        msg = f"{resource} not found"
        if resource_id:
            msg = f"{resource} #{resource_id} not found"
        super().__init__(code="NOT_FOUND", message=msg, status_code=404)


class ValidationError(AppError):
    def __init__(self, message: str, detail: str = ""):
        super().__init__(code="VALIDATION_ERROR", message=message, detail=detail, status_code=422)


class ConflictError(AppError):
    def __init__(self, message: str, detail: str = ""):
        super().__init__(code="CONFLICT", message=message, detail=detail, status_code=409)
