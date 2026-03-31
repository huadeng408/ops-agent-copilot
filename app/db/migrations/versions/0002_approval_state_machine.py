revision = '0002_approval_state_machine'
down_revision = '0001_init'
branch_labels = None
depends_on = None

from alembic import op
import sqlalchemy as sa


def upgrade() -> None:
    with op.batch_alter_table('approvals') as batch_op:
        batch_op.add_column(sa.Column('idempotency_key', sa.String(length=96), nullable=True))
        batch_op.add_column(sa.Column('executed_at', sa.DateTime(), nullable=True))
        batch_op.add_column(sa.Column('execution_result', sa.JSON(), nullable=True))
        batch_op.add_column(sa.Column('execution_error', sa.String(length=255), nullable=True))
        batch_op.add_column(sa.Column('version', sa.Integer(), nullable=False, server_default='1'))

    op.execute("UPDATE approvals SET idempotency_key = approval_no WHERE idempotency_key IS NULL")
    op.execute(
        """
        UPDATE approvals
        SET status = 'executed',
            executed_at = COALESCE(executed_at, approved_at)
        WHERE status = 'approved'
          AND id IN (
            SELECT approval_id
            FROM ticket_actions
            WHERE approval_id IS NOT NULL
          )
        """
    )

    with op.batch_alter_table('approvals') as batch_op:
        batch_op.alter_column('idempotency_key', existing_type=sa.String(length=96), nullable=False)
        batch_op.create_unique_constraint('uq_approvals_idempotency_key', ['idempotency_key'])
        batch_op.create_index('idx_approvals_idempotency_key', ['idempotency_key'], unique=False)

    with op.batch_alter_table('ticket_actions') as batch_op:
        batch_op.create_unique_constraint('uk_ticket_actions_approval_id', ['approval_id'])


def downgrade() -> None:
    with op.batch_alter_table('ticket_actions') as batch_op:
        batch_op.drop_constraint('uk_ticket_actions_approval_id', type_='unique')

    with op.batch_alter_table('approvals') as batch_op:
        batch_op.drop_index('idx_approvals_idempotency_key')
        batch_op.drop_constraint('uq_approvals_idempotency_key', type_='unique')
        batch_op.drop_column('version')
        batch_op.drop_column('execution_error')
        batch_op.drop_column('execution_result')
        batch_op.drop_column('executed_at')
        batch_op.drop_column('idempotency_key')
