import React, { useState } from 'react';
import { TextField, Box } from '@mui/material';

const ChatInput = ({ onSendMessage }) => {
  const [message, setMessage] = useState('');

  const handleSubmit = (event) => {
    event.preventDefault();
    if (message.trim() !== '') {
      onSendMessage(message);
      setMessage('');
    }
  };

  return (
    <Box
     component="form"
     onSubmit={handleSubmit}
    >
      <TextField
        fullWidth
        placeholder="Type your message..."
        variant="outlined"
        value={message}
        onChange={(e) => setMessage(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey) {
            handleSubmit(e);
          }
         }
        }
      />
    </Box>
  );
};

export default ChatInput;