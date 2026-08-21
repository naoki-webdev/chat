package main

import "context"

func (r *postgresRepository) ListUsers(ctx context.Context) ([]PublicUser, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,name,handle,initials,color FROM users WHERE is_bot=false ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]PublicUser, 0)
	for rows.Next() {
		var user PublicUser
		if err := rows.Scan(&user.ID, &user.Name, &user.Handle, &user.Initials, &user.Color); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (r *postgresRepository) ListChannelMembers(ctx context.Context, channelID string) ([]ChannelMember, error) {
	rows, err := r.pool.Query(ctx, `SELECT u.id,u.name,u.handle,u.initials,u.color,cm.role,u.is_bot FROM channel_members cm JOIN users u ON u.id=cm.user_id WHERE cm.channel_id=$1 ORDER BY u.name,u.id`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := make([]ChannelMember, 0)
	for rows.Next() {
		var member ChannelMember
		if err := rows.Scan(&member.ID, &member.Name, &member.Handle, &member.Initials, &member.Color, &member.Role, &member.IsBot); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}
