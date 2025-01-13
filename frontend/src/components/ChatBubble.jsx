import React from 'react';
import { Paper, Typography, Box } from '@mui/material';
import { useSpring, animated } from 'react-spring';

const ChatBubble = ({ text, isUser, fadeIn }) => {
    const springProps = useSpring({
        opacity: fadeIn && !isUser ? 1 : 1,
        from: { opacity: fadeIn && !isUser ? 0 : 1 },
        config: { duration: 500 },
    });

    return (
        <animated.div style={springProps}>
            <Box
                display="flex"
                justifyContent={isUser ? 'flex-end' : 'flex-start'}
                mb={2}
            >
                <Paper
                    elevation={3}
                    style={{
                        padding: '10px 15px',
                        borderRadius: isUser ? '15px 0 15px 15px' : '0 15px 15px 15px',
                        backgroundColor: isUser ? '#DCF8C6' : '#fff',
                        maxWidth: '70%',
                    }}
                >
                    <Typography variant="body1">
                        {text}
                    </Typography>
                </Paper>
            </Box>
        </animated.div>
    );
};

export default ChatBubble;