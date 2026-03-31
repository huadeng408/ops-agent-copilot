class AppError(Exception):
    """Base application error."""


class NotFoundError(AppError):
    """Raised when a resource is not found."""


class PermissionDeniedError(AppError):
    """Raised when current user lacks permission."""


class ValidationAppError(AppError):
    """Raised when input payload is invalid."""


class ConflictError(AppError):
    """Raised when resource state conflicts with operation."""
