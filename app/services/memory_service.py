from app.repositories.session_repo import SessionRepository


class MemoryService:
    def __init__(self, session_repo: SessionRepository, keep_recent_message_count: int = 8) -> None:
        self.session_repo = session_repo
        self.keep_recent_message_count = keep_recent_message_count

    async def build_context(self, session_id: str) -> dict:
        messages = await self.session_repo.list_recent_messages(session_id, self.keep_recent_message_count)
        return {
            'messages': [
                {
                    'role': message.role,
                    'text': str(message.content.get('text', '')),
                }
                for message in messages
            ],
            'summary': None,
        }

    async def maybe_update_summary(self, session_id: str) -> None:
        messages = await self.session_repo.list_recent_messages(session_id, self.keep_recent_message_count + 6)
        if len(messages) <= self.keep_recent_message_count:
            return
        older = messages[:-self.keep_recent_message_count]
        summary = ' | '.join(f"{message.role}:{str(message.content.get('text', ''))[:80]}" for message in older)
        await self.session_repo.update_summary(session_id, summary[:2000])
