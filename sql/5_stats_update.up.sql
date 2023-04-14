ALTER TABLE gym
  MODIFY COLUMN cell_id bigint default NULL;

ALTER TABLE pokemon
    MODIFY COLUMN cell_id bigint default NULL;

ALTER TABLE pokestop
    MODIFY COLUMN cell_id bigint default NULL;

alter table pokemon
    drop index `ix_expire_timestamp`;

alter table pokemon
    add index `ix_expire_timestamp_verified` (`expire_timestamp`, `expire_timestamp_verified`);

ALTER TABLE pokemon_history
    ADD KEY `expire_timestamp` (`expire_timestamp`);
