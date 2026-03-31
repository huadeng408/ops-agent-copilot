from app.services.report_service import ReportService


async def run_daily_report(report_service: ReportService) -> dict:
    return await report_service.generate_daily_report()
