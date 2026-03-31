from pydantic import BaseModel


class ReportSection(BaseModel):
    title: str
    content: str


class ReportResponse(BaseModel):
    title: str
    sections: list[ReportSection]
