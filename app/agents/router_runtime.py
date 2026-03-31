from app.agents.router import MessageRouter


class SingleAgentRouter:
    def __init__(self) -> None:
        self.router = MessageRouter()

    def route(self, message: str):
        return self.router.route(message)
